// Package api serves the authenticated native desktop API. It listens on the
// loopback address only and defends against other websites and local
// processes: a per-run secret cookie, a required custom header, JSON-only
// POSTs, a Host allow-list (DNS rebinding), and a strict Content-Security-Policy.
package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"spearhead/internal/identity"
	"spearhead/internal/lan"
	"spearhead/internal/node"
	"spearhead/internal/protocol"
	"spearhead/internal/safety"
	"spearhead/internal/store"
)

const nearbyWindow = 20 // seconds a beacon counts as "nearby now"

type Server struct {
	N      *node.Node
	Token  string
	Port   int
	LAN    *lan.Monitor
	OnQuit func()
}

func New(n *node.Node) *Server {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return &Server{N: n, Token: hex.EncodeToString(b), LAN: lan.New()}
}

// Serve listens on 127.0.0.1 (trying up to 10 ports from startPort) and
// returns the sign-in URL to open in a browser.
func (s *Server) Serve(ctx context.Context, startPort int) (string, error) {
	var ln net.Listener
	var err error
	for p := startPort; p < startPort+10; p++ {
		if ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p)); err == nil {
			break
		}
	}
	if ln == nil {
		return "", err
	}
	s.Port = ln.Addr().(*net.TCPAddr).Port
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	go srv.Serve(ln)
	go func() { <-ctx.Done(); srv.Close() }()
	return fmt.Sprintf("http://127.0.0.1:%d/?t=%s", s.Port, s.Token), nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.root)
	mux.HandleFunc("GET /api/ping", func(w http.ResponseWriter, r *http.Request) {
		if hostOK(r) {
			w.Write([]byte("spearhead"))
		}
	})
	mux.HandleFunc("GET /api/state", s.api(s.state))
	mux.HandleFunc("GET /api/thread", s.api(s.thread))
	mux.HandleFunc("GET /api/safety", s.api(s.safety))
	mux.HandleFunc("POST /api/send", s.api(s.send))
	mux.HandleFunc("POST /api/reveal", s.api(s.reveal))
	mux.HandleFunc("POST /api/message/delete", s.api(s.deleteMessage))
	mux.HandleFunc("POST /api/lookup", s.api(s.lookup))
	mux.HandleFunc("POST /api/add", s.api(s.add))
	mux.HandleFunc("POST /api/contact", s.api(s.contact))
	mux.HandleFunc("POST /api/settings", s.api(s.settings))
	mux.HandleFunc("GET /api/network", s.api(s.network))
	mux.HandleFunc("GET /api/activity", s.api(s.activity))
	mux.HandleFunc("POST /api/network/scan", s.api(s.scan))
	mux.HandleFunc("POST /api/files", s.fileAPI(s.upload, true))
	mux.HandleFunc("GET /api/file", s.fileAPI(s.download, false))
	mux.HandleFunc("POST /api/desktop/quit", s.api(s.quit))
	return mux
}

// ---------- security plumbing ----------

func secHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Cache-Control", "no-store")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
}

func hostOK(r *http.Request) bool {
	h, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		h = r.Host
	}
	// The app always opens a numeric loopback URL. Do not accept localhost:
	// browser or hosts-file resolution would otherwise widen this boundary.
	return h == "127.0.0.1" || h == "::1"
}

func (s *Server) cookieOK(r *http.Request) bool {
	c, err := r.Cookie("spearhead")
	return err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.Token)) == 1
}

func (s *Server) root(w http.ResponseWriter, r *http.Request) {
	secHeaders(w)
	if !hostOK(r) {
		http.Error(w, "forbidden", 403)
		return
	}
	if t := r.URL.Query().Get("t"); t != "" && subtle.ConstantTimeCompare([]byte(t), []byte(s.Token)) == 1 {
		http.SetCookie(w, &http.Cookie{Name: "spearhead", Value: s.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !s.cookieOK(r) {
		jerr(w, http.StatusUnauthorized, "The native desktop requires its private session.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"application": "SpearHead", "interface": "Qt Quick/QML"})
}

func (s *Server) api(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secHeaders(w)
		switch {
		case !hostOK(r):
			jerr(w, 403, "forbidden")
		case !s.cookieOK(r):
			jerr(w, 401, "You are not signed in. Use the link SpearHead opened, or run spearhead again.")
		case r.Header.Get("X-SpearHead") != "1":
			jerr(w, 403, "forbidden")
		case r.Method == http.MethodPost && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json"):
			jerr(w, 415, "expected JSON")
		default:
			h(w, r)
		}
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func jerr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		jerr(w, 400, "That request could not be read.")
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		jerr(w, 400, "That request must contain one JSON value.")
		return false
	}
	return true
}

// ---------- views ----------

type contactView struct {
	Pub         string `json:"pub"`
	Label       string `json:"label"`
	TheirName   string `json:"their_name"`
	Fingerprint string `json:"fingerprint"`
	Status      string `json:"status"`
	Verified    bool   `json:"verified"`
	Unread      int    `json:"unread"`
	LastSeen    int64  `json:"last_seen"`
	Nearby      bool   `json:"nearby"`
	Preview     string `json:"preview"`
	LastTS      int64  `json:"last_ts"`
	Waiting     int    `json:"waiting"`
	Warn        string `json:"warn,omitempty"`
	Device      string `json:"device"`
	Platform    string `json:"platform"`
	Address     string `json:"address"`
	Notes       string `json:"notes"`
	FirstSeen   int64  `json:"first_seen"`
}

type msgView struct {
	ID     string             `json:"id"`
	Dir    string             `json:"dir"`
	TS     int64              `json:"ts"`
	Kind   string             `json:"kind"`
	Body   string             `json:"body"`
	State  string             `json:"state"`
	Reason string             `json:"reason,omitempty"`
	Burn   bool               `json:"burn"`
	Hidden bool               `json:"hidden"`
	Gap    bool               `json:"gap"`
	File   *protocol.FileInfo `json:"file,omitempty"`
}

func viewMsg(m *store.Message) msgView {
	v := msgView{ID: m.Env.ID, Dir: m.Dir, TS: m.Env.TS, Kind: m.Env.Kind, Body: m.Env.Body, State: m.State,
		Reason: m.Reason, Burn: m.Env.Burn, Gap: m.Gap}
	if m.Env.Kind == protocol.KindFile {
		if f, err := protocol.ParseFile(m.Env.Body); err == nil {
			v.File = &f
			v.Body = "File: " + f.Name
		}
	}
	if m.Env.Burn && (m.Dir == store.DirIn || m.State != store.StateDelivered) {
		v.Body, v.Hidden = "", true // read-once content is never sent to the page until revealed
	}
	return v
}

func fingerprint(pub string) string {
	k, err := identity.Decode(pub)
	if err != nil {
		return ""
	}
	return identity.Fingerprint(k)
}

func realMessages(ms []*store.Message) (out []*store.Message) {
	for _, m := range ms {
		if m.Env.Kind != protocol.KindRequest {
			out = append(out, m)
		}
	}
	return
}

func (s *Server) state(w http.ResponseWriter, r *http.Request) {
	n := s.N
	cfg := n.Settings()
	port := n.Port()
	if port == 0 {
		port = cfg.ListenPort
	}
	now := time.Now().Unix()
	near := map[string]*store.Peer{}
	for _, p := range n.St.Peers() {
		if now-p.Seen <= nearbyWindow {
			near[p.Pub] = p
		}
	}
	unread := map[string]int{}
	for _, m := range n.St.Unread() {
		if m.Env.Kind != protocol.KindRequest {
			unread[m.Thread]++
		}
	}
	all := n.St.ListContacts()
	contacts, requests := []contactView{}, []contactView{}
	for _, c := range all {
		v := contactView{Pub: c.Pub, Label: c.Label, TheirName: c.TheirName, Fingerprint: fingerprint(c.Pub), Status: c.Status,
			Verified: c.Verified, Unread: unread[c.Pub], LastSeen: c.LastSeen, Device: c.Device, Platform: c.Platform, Address: c.LastAddr, Notes: c.Notes, FirstSeen: c.FirstSeen}
		_, v.Nearby = near[c.Pub]
		if p := near[c.Pub]; p != nil {
			v.TheirName = p.Name
			if p.Device != "" {
				v.Device, v.Platform = p.Device, p.Platform
			}
			if p.Seen > v.LastSeen {
				v.LastSeen = p.Seen
			}
		}
		msgs := realMessages(n.St.Thread(c.Pub))
		v.Waiting = len(msgs)
		if len(msgs) > 0 {
			last := msgs[len(msgs)-1]
			v.LastTS = last.Env.TS
			if !last.Env.Burn {
				v.Preview = last.Env.Body
				if last.Env.Kind == protocol.KindFile {
					if f, err := protocol.ParseFile(last.Env.Body); err == nil {
						v.Preview = "File: " + f.Name
					}
				}
				if len(v.Preview) > 80 {
					v.Preview = v.Preview[:80]
				}
			}
		}
		if c.Status == store.StatusPending {
			for _, q := range all {
				if q.Status != store.StatusPending && (strings.EqualFold(c.TheirName, q.Label) || strings.EqualFold(c.TheirName, q.TheirName)) {
					v.Warn = fmt.Sprintf("This person uses the same name as your contact %q but has a different identity. It could be a reinstall, or someone pretending. Check with them before accepting.", q.Label)
				}
			}
			requests = append(requests, v)
		} else {
			contacts = append(contacts, v)
		}
	}
	type nv struct {
		Pub         string `json:"pub"`
		Name        string `json:"name"`
		Fingerprint string `json:"fingerprint"`
		Addr        string `json:"addr"`
		Known       bool   `json:"known"`
	}
	nearby := []nv{}
	for _, p := range near {
		_, err := n.St.GetContact(p.Pub)
		nearby = append(nearby, nv{p.Pub, p.Name, fingerprint(p.Pub), p.Addr, err == nil})
	}
	failed := 0
	for _, c := range contacts {
		for _, m := range n.St.Thread(c.Pub) {
			if m.Dir == store.DirOut && m.State == store.StateFailed {
				failed++
			}
		}
	}
	writeJSON(w, 200, map[string]any{
		"me":        selfView(s),
		"contacts":  contacts,
		"requests":  requests,
		"nearby":    nearby,
		"outbox":    len(n.St.Outbox()),
		"events":    n.ActivityVersion(),
		"failed":    failed,
		"listening": n.Port() > 0,
		"port":      port,
		"settings":  map[string]any{"retention_days": cfg.DefaultRetentionDays, "secret_detection": cfg.SecretDetection},
	})
}

func (s *Server) contactOf(pub string) (*store.Contact, error) {
	if _, err := identity.Decode(pub); err != nil {
		return nil, errors.New("Unknown person.")
	}
	c, err := s.N.St.GetContact(pub)
	if err != nil {
		return nil, errors.New("Unknown person.")
	}
	return c, nil
}

func (s *Server) acceptedOf(pub string) (*store.Contact, error) {
	c, err := s.contactOf(pub)
	if err != nil {
		return nil, err
	}
	if c.Status != store.StatusAccepted {
		return nil, errors.New("You need to accept this person before messaging them.")
	}
	return c, nil
}

func (s *Server) thread(w http.ResponseWriter, r *http.Request) {
	c, err := s.contactOf(r.URL.Query().Get("c"))
	if err != nil {
		jerr(w, 404, err.Error())
		return
	}
	out := []msgView{}
	msgs := realMessages(s.N.St.Thread(c.Pub))
	if len(msgs) > 200 {
		msgs = msgs[len(msgs)-200:]
	}
	for _, m := range msgs {
		if c.Status == store.StatusAccepted && m.Dir == store.DirIn && m.State == store.StateUnread && !m.Env.Burn {
			m.State = store.StateRead
			s.N.St.SaveMessage(m)
		}
		out = append(out, viewMsg(m))
	}
	writeJSON(w, 200, map[string]any{"messages": out})
}

func (s *Server) safety(w http.ResponseWriter, r *http.Request) {
	c, err := s.contactOf(r.URL.Query().Get("c"))
	if err != nil {
		jerr(w, 404, err.Error())
		return
	}
	pub, _ := identity.Decode(c.Pub)
	writeJSON(w, 200, map[string]any{"code": identity.SafetyCode(s.N.ID.Pub, pub), "verified": c.Verified})
}

// ---------- actions ----------

func (s *Server) deliver(w http.ResponseWriter, c *store.Contact, body string, force bool, o node.SendOpts) {
	body = strings.TrimSpace(body)
	if body == "" {
		jerr(w, 400, "There is nothing to send.")
		return
	}
	if len(body) > protocol.MaxBody {
		jerr(w, 400, "That message is too long.")
		return
	}
	if s.N.Settings().SecretDetection && !force {
		if f := safety.Scan(body); len(f) > 0 {
			writeJSON(w, 409, map[string]any{"error": "secrets", "findings": f})
			return
		}
	}
	m, err := s.N.Send(c, body, o)
	if err != nil {
		jerr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, viewMsg(m))
}

func (s *Server) send(w http.ResponseWriter, r *http.Request) {
	var q struct {
		C     string `json:"c"`
		Body  string `json:"body"`
		Burn  bool   `json:"burn"`
		Force bool   `json:"force"`
	}
	if !decode(w, r, &q) {
		return
	}
	c, err := s.acceptedOf(q.C)
	if err != nil {
		jerr(w, 400, err.Error())
		return
	}
	s.deliver(w, c, q.Body, q.Force, node.SendOpts{Burn: q.Burn})
}

func (s *Server) reveal(w http.ResponseWriter, r *http.Request) {
	var q struct{ C, ID string }
	if !decode(w, r, &q) {
		return
	}
	c, err := s.acceptedOf(q.C)
	if err != nil {
		jerr(w, 400, err.Error())
		return
	}
	body, err := s.N.St.OpenReadOnce(c.Pub, q.ID)
	if err != nil {
		jerr(w, 404, "That message is no longer available. Read-once messages can only be opened one time.")
		return
	}
	writeJSON(w, 200, map[string]any{"body": body})
}

var hostPortRe = regexp.MustCompile(`^[^\s:]+:\d{1,5}$|^\[[0-9a-fA-F:]+\]:\d{1,5}$`)

func (s *Server) lookup(w http.ResponseWriter, r *http.Request) {
	var q struct{ Target string }
	if !decode(w, r, &q) {
		return
	}
	t := strings.TrimSpace(q.Target)
	type found struct {
		Pub         string `json:"pub"`
		Name        string `json:"name"`
		Fingerprint string `json:"fingerprint"`
		Addr        string `json:"addr"`
		Known       bool   `json:"known"`
		Status      string `json:"status,omitempty"`
	}
	reply := func(pub []byte, name, addr string) {
		p := identity.Encode(pub)
		c, err := s.N.St.GetContact(p)
		f := found{Pub: p, Name: name, Fingerprint: identity.Fingerprint(pub), Addr: addr, Known: err == nil}
		if err == nil {
			f.Status = c.Status
		}
		writeJSON(w, 200, f)
	}
	switch {
	case node.IsInvite(t):
		pub, name, addrs, err := node.ParseInvite(t)
		if err != nil {
			jerr(w, 400, "That invite code doesn't look right. Ask them to copy it again.")
			return
		}
		addr := ""
		for _, a := range addrs {
			if p, _, err := s.N.Probe(a); err == nil && string(p) == string(pub) {
				addr = a
				break
			}
		}
		if addr == "" && len(addrs) > 0 {
			addr = addrs[0]
		}
		reply(pub, name, addr)
	case hostPortRe.MatchString(t):
		pub, name, err := s.N.Probe(t)
		if err != nil {
			jerr(w, 502, "Couldn't reach that address. Check it's right, that SpearHead is running there, and that a firewall isn't blocking it.")
			return
		}
		reply(pub, name, t)
	default:
		jerr(w, 400, "Paste an invite code (starts with rkt1:) or an address like 10.0.0.5:7777.")
	}
}

func (s *Server) add(w http.ResponseWriter, r *http.Request) {
	var q struct{ Pub, Label, Name, Addr string }
	if !decode(w, r, &q) {
		return
	}
	pub, err := identity.Decode(q.Pub)
	if err != nil {
		jerr(w, 400, "Unknown person.")
		return
	}
	if q.Addr != "" {
		if _, _, err := net.SplitHostPort(q.Addr); err != nil {
			jerr(w, 400, "That address doesn't look right.")
			return
		}
	}
	c, err := s.N.AddContact(pub, identity.CleanName(q.Label), identity.CleanName(q.Name), q.Addr)
	if err != nil {
		jerr(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"pub": c.Pub, "label": c.Label})
}

func (s *Server) contact(w http.ResponseWriter, r *http.Request) {
	var q struct{ C, Action, Label, Notes, Address string }
	if !decode(w, r, &q) {
		return
	}
	c, err := s.contactOf(q.C)
	if err != nil {
		jerr(w, 404, err.Error())
		return
	}
	switch q.Action {
	case "accept":
		if c.Status == store.StatusBlocked {
			jerr(w, 400, "Unblock this person first.")
			return
		}
		c.Status = store.StatusAccepted
	case "unblock":
		c.Status = store.StatusAccepted
	case "block":
		c.Status = store.StatusBlocked
	case "verify":
		c.Verified = true
	case "edit":
		if q.Address != "" {
			_, p, err := net.SplitHostPort(q.Address)
			port, e := strconv.Atoi(p)
			if err != nil || e != nil || port < 1 || port > 65535 {
				jerr(w, 400, "Enter an address such as 192.168.1.8:7777.")
				return
			}
		}
		if len(q.Notes) > 4096 {
			jerr(w, 400, "Notes are too long.")
			return
		}
		c.Notes, c.LastAddr = q.Notes, q.Address
		fallthrough
	case "rename":
		l := identity.CleanName(q.Label)
		if l == "" {
			jerr(w, 400, "Enter a name.")
			return
		}
		c.Label = l
	case "decline", "remove":
		if err := s.N.St.DeleteContact(c.Pub); err != nil {
			jerr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true})
		return
	default:
		jerr(w, 400, "Unknown action.")
		return
	}
	if err := s.N.St.UpdateContact(c.Pub, func(fresh *store.Contact) {
		switch q.Action {
		case "accept", "unblock", "block":
			fresh.Status = c.Status
		case "verify":
			fresh.Verified = c.Verified
		case "rename":
			fresh.Label = c.Label
		case "edit":
			fresh.Label, fresh.Notes, fresh.LastAddr = c.Label, c.Notes, c.LastAddr
		}
	}); err != nil {
		jerr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	var q struct {
		Name            string `json:"name"`
		RetentionDays   int    `json:"retention_days"`
		SecretDetection bool   `json:"secret_detection"`
	}
	if !decode(w, r, &q) {
		return
	}
	if q.RetentionDays < 0 || q.RetentionDays > 3650 {
		jerr(w, 400, "Keep-for days must be between 0 and 3650.")
		return
	}
	if name := identity.CleanName(q.Name); name != "" && name != s.N.ID.DisplayName() {
		if err := s.N.ID.SetName(s.N.Home, name); err != nil {
			jerr(w, 500, err.Error())
			return
		}
	}
	if err := s.N.SaveSettings(q.RetentionDays, q.SecretDetection); err != nil {
		jerr(w, 500, err.Error())
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
}
