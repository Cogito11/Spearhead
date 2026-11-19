package api

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"errors"
	"spearhead/internal/identity"
	"spearhead/internal/lan"
	"spearhead/internal/protocol"
)

func (s *Server) network(w http.ResponseWriter, r *http.Request) {
	snap := s.LAN.Snapshot()
	peers := []lan.Device{}
	for _, p := range s.N.St.Peers() {
		if time.Now().Unix()-p.Seen > 90 {
			continue
		}
		host, _, err := net.SplitHostPort(p.Addr)
		if err != nil {
			continue
		}
		peers = append(peers, lan.Device{ID: p.Pub, IP: host, Name: p.Device, Platform: p.Platform, Pub: p.Pub, BroadcastName: p.Name, Address: p.Addr, Status: "SpearHead online", Source: "Signed SpearHead announcement", Seen: p.Seen, Kind: "Computer", Confidence: "Reported", Identification: "Signed SpearHead device information"})
	}
	snap.Devices = lan.Inventory(snap, peers, identity.Encode(s.N.ID.Pub), s.N.ID.DisplayName())

	writeJSON(w, 200, snap)
}

func (s *Server) scan(w http.ResponseWriter, r *http.Request) {
	var q struct{}
	if !decode(w, r, &q) {
		return
	}
	started := s.LAN.StartScan()
	status := s.LAN.Snapshot()
	writeJSON(w, 202, map[string]any{"started": started, "scanning": status.Scanning, "identifying": status.Identifying})
}

// File endpoints retain cookie/Host protection and require a custom header for
// uploads. Downloads allow normal links; SameSite cookies and same-origin
// response policy prevent other websites reading attachments.
func (s *Server) fileAPI(h http.HandlerFunc, upload bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secHeaders(w)
		if !hostOK(r) {
			jerr(w, 403, "forbidden")
			return
		}
		if !s.cookieOK(r) {
			jerr(w, 401, "Use your private SpearHead link.")
			return
		}
		if upload && (r.Header.Get("X-SpearHead") != "1" || r.Header.Get("Content-Type") != "application/octet-stream") {
			jerr(w, 415, "expected a file upload")
			return
		}
		h(w, r)
	}
}

func (s *Server) upload(w http.ResponseWriter, r *http.Request) {
	c, err := s.acceptedOf(r.URL.Query().Get("c"))
	if err != nil {
		jerr(w, 400, err.Error())
		return
	}
	if r.ContentLength > protocol.MaxFileSize {
		jerr(w, 413, "Files must be 256 MiB or smaller.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, protocol.MaxFileSize)
	m, err := s.N.SendFile(c, r.URL.Query().Get("name"), r.Body)
	if err != nil {
		jerr(w, 400, err.Error())
		return
	}
	writeJSON(w, 202, viewMsg(m))
}

func (s *Server) download(w http.ResponseWriter, r *http.Request) {
	c, err := s.contactOf(r.URL.Query().Get("c"))
	if err != nil {
		jerr(w, 404, err.Error())
		return
	}
	m := s.N.St.GetMessage(c.Pub, r.URL.Query().Get("id"))
	if m == nil || m.Env.Kind != protocol.KindFile {
		jerr(w, 404, "That file is no longer available.")
		return
	}
	info, err := protocol.ParseFile(m.Env.Body)
	if err != nil {
		jerr(w, 404, "That file is unavailable.")
		return
	}
	f, err := os.Open(s.N.St.FilePath(c.Pub, m.Env.ID))
	if err != nil {
		jerr(w, 404, "That file is no longer available.")
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		jerr(w, 500, "Could not read the file.")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", escapeFileName(info.Name)))
	w.Header().Set("Content-Length", strconv.FormatInt(st.Size(), 10))
	http.ServeContent(w, r, info.Name, st.ModTime(), f)
}

func escapeFileName(s string) string {
	var out strings.Builder
	for _, b := range []byte(s) {
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || strings.ContainsRune("-._", rune(b)) {
			out.WriteByte(b)
		} else {
			fmt.Fprintf(&out, "%%%02X", b)
		}
	}
	return out.String()
}

func (s *Server) quit(w http.ResponseWriter, r *http.Request) {
	var q struct{}
	if !decode(w, r, &q) {
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
	if flush, ok := w.(http.Flusher); ok {
		flush.Flush()
	}
	if s.OnQuit != nil {
		go s.OnQuit()
	}
}

func selfView(s *Server) map[string]any {
	device, platform := identity.LocalDevice()
	return map[string]any{"pub": identity.Encode(s.N.ID.Pub), "name": s.N.ID.DisplayName(), "label": s.N.ID.DisplayName(), "their_name": s.N.ID.DisplayName(), "device": device, "platform": platform, "fingerprint": identity.Fingerprint(s.N.ID.Pub), "invite": s.N.InviteCode(), "addresses": s.N.LocalAddresses(), "status": "self"}
}

func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"events": s.N.ActivityVersion()})
}

// deleteMessage is a local history action, authenticated like sending a message.
func (s *Server) deleteMessage(w http.ResponseWriter, r *http.Request) {
	var q struct {
		C  string `json:"c"`
		ID string `json:"id"`
	}
	if !decode(w, r, &q) {
		return
	}
	c, err := s.contactOf(q.C)
	if err != nil {
		jerr(w, 404, err.Error())
		return
	}
	if len(q.ID) != 32 || strings.Trim(q.ID, "0123456789abcdef") != "" {
		jerr(w, 400, "Invalid message.")
		return
	}
	if m := s.N.St.GetMessage(c.Pub, q.ID); m != nil && m.Env.Kind == protocol.KindRequest {
		jerr(w, 400, "Use the contact actions for contact requests.")
		return
	}
	if err = s.N.St.DeleteLocalMessage(c.Pub, q.ID); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			jerr(w, 404, "That message is no longer available.")
		} else {
			jerr(w, 500, "Could not delete that message.")
		}
		return
	}
	writeJSON(w, 200, map[string]any{"deleted": true, "scope": "local"})
}
