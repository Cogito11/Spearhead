package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"spearhead/internal/identity"
	"spearhead/internal/lan"
	"spearhead/internal/node"
	"spearhead/internal/store"
)

func mk(t *testing.T, name string) *node.Node {
	t.Helper()
	home := t.TempDir()
	if _, err := identity.Create(home, name); err != nil {
		t.Fatal(err)
	}
	n, err := node.New(home)
	if err != nil {
		t.Fatal(err)
	}
	n.Cfg.ListenPort, n.Cfg.DefaultRetentionDays = 0, 0
	node.DialTimeout = time.Second
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := n.Serve(ctx); err != nil {
		t.Fatal(err)
	}
	return n
}

type client struct {
	t  *testing.T
	c  *http.Client
	ts *httptest.Server
}

func ui(t *testing.T, n *node.Node) (*client, *Server) {
	s := New(n)
	s.LAN = lan.NewPassive()
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	r, err := c.Get(ts.URL + "/?t=" + s.Token)
	if err != nil || r.StatusCode != 303 {
		t.Fatalf("sign-in failed: %v %v", r, err)
	}
	return &client{t, c, ts}, s
}

func (c *client) do(method, path string, body any, hdr map[string]string) (int, map[string]any) {
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, c.ts.URL+path, rd)
	req.Header.Set("X-SpearHead", "1")
	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range hdr {
		if k == "Host" {
			req.Host = v // net/http ignores a Host entry in Header
		} else if v == "" {
			req.Header.Del(k)
		} else {
			req.Header.Set(k, v)
		}
	}
	r, err := c.c.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer r.Body.Close()
	var out map[string]any
	json.NewDecoder(r.Body).Decode(&out)
	return r.StatusCode, out
}

func befriend(t *testing.T, a, b *node.Node) string {
	c, err := a.AddContact(b.ID.Pub, "bob", "Bob", fmt.Sprintf("127.0.0.1:%d", b.Port()))
	if err != nil {
		t.Fatal(err)
	}
	bc, err := b.St.GetContact(identity.Encode(a.ID.Pub))
	if err != nil {
		t.Fatal("request missing", err)
	}
	bc.Status = store.StatusAccepted
	b.St.SaveContact(bc)
	return c.Pub
}

func TestAuthAndHeaders(t *testing.T) {
	n := mk(t, "Alex")
	c, s := ui(t, n)
	if code, _ := c.do("GET", "/api/state", nil, nil); code != 200 {
		t.Fatalf("signed-in state: %d", code)
	}
	if code, _ := c.do("GET", "/api/state", nil, map[string]string{"X-SpearHead": ""}); code != 403 {
		t.Fatalf("missing custom header must be refused, got %d", code)
	}
	if code, _ := c.do("GET", "/api/state", nil, map[string]string{"Host": "evil.example"}); code != 403 {
		t.Fatalf("foreign Host (DNS rebinding) must be refused, got %d", code)
	}
	if code, _ := c.do("POST", "/api/send", map[string]any{}, map[string]string{"Content-Type": "text/plain"}); code != 415 {
		t.Fatalf("non-JSON POST must be refused, got %d", code)
	}
	anon := &http.Client{}
	r, _ := anon.Get(c.ts.URL + "/api/state")
	if r.StatusCode != 401 {
		t.Fatalf("no cookie must be 401, got %d", r.StatusCode)
	}
	r, _ = anon.Get(c.ts.URL + "/")
	if r.StatusCode != 401 {
		t.Fatalf("page without cookie must not load the app, got %d", r.StatusCode)
	}
	r, _ = anon.Get(c.ts.URL + "/?t=wrong")
	if r.StatusCode != 401 {
		t.Fatalf("wrong token accepted: %d", r.StatusCode)
	}
	r, _ = c.c.Get(c.ts.URL + "/")
	if r.StatusCode != 200 || !strings.Contains(r.Header.Get("Content-Security-Policy"), "script-src 'self'") {
		t.Fatalf("app page must load with strict CSP: %d %q", r.StatusCode, r.Header.Get("Content-Security-Policy"))
	}
	_ = s
}

func TestSendSecretGuardAndDelivery(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	pub := befriend(t, a, b)
	c, _ := ui(t, a)
	code, out := c.do("POST", "/api/send", map[string]any{"c": pub, "body": "the password is Hunter2Hunter2"}, nil)
	if code != 409 || out["error"] != "secrets" {
		t.Fatalf("secret must trigger a warning: %d %v", code, out)
	}
	for _, m := range b.St.Unread() {
		if m.Env.Kind != "request" {
			t.Fatal("nothing should have been delivered before confirmation")
		}
	}
	code, out = c.do("POST", "/api/send", map[string]any{"c": pub, "body": "the password is Hunter2Hunter2", "force": true}, nil)
	if code != 200 || out["state"] != "delivered" {
		t.Fatalf("forced send: %d %v", code, out)
	}
	code, out = c.do("POST", "/api/send", map[string]any{"c": pub, "body": "   "}, nil)
	if code != 400 {
		t.Fatalf("blank message must be refused: %d", code)
	}
}

func TestBurnRevealOnce(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	pubOnA := befriend(t, a, b)
	if _, err := a.Send(mustContact(t, a, pubOnA), "temp pw 123", node.SendOpts{Burn: true}); err != nil {
		t.Fatal(err)
	}
	cb, _ := ui(t, b)
	alexOnB := identity.Encode(a.ID.Pub)
	_, th := cb.do("GET", "/api/thread?c="+alexOnB, nil, nil)
	msgs := th["messages"].([]any)
	m := msgs[len(msgs)-1].(map[string]any)
	if m["hidden"] != true || m["body"] != "" {
		t.Fatalf("burn body must not be sent to the page before reveal: %v", m)
	}
	code, out := cb.do("POST", "/api/reveal", map[string]any{"c": alexOnB, "id": m["id"]}, nil)
	if code != 200 || out["body"] != "temp pw 123" {
		t.Fatalf("reveal: %d %v", code, out)
	}
	if code, _ := cb.do("POST", "/api/reveal", map[string]any{"c": alexOnB, "id": m["id"]}, nil); code != 404 {
		t.Fatalf("second reveal must fail, got %d", code)
	}
}

func mustContact(t *testing.T, n *node.Node, pub string) *store.Contact {
	c, err := n.St.GetContact(pub)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLookupAddAndRequestsFlow(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	ca, _ := ui(t, a)
	cb, _ := ui(t, b)
	addr := fmt.Sprintf("127.0.0.1:%d", b.Port())
	code, out := ca.do("POST", "/api/lookup", map[string]any{"target": addr}, nil)
	if code != 200 || out["name"] != "Bob" || out["fingerprint"] == "" {
		t.Fatalf("lookup by address: %d %v", code, out)
	}
	if code, _ := ca.do("POST", "/api/lookup", map[string]any{"target": "not a thing"}, nil); code != 400 {
		t.Fatalf("garbage lookup must be 400, got %d", code)
	}
	if code, _ := ca.do("POST", "/api/lookup", map[string]any{"target": "127.0.0.1:1"}, nil); code != 502 {
		t.Fatalf("unreachable address must give a friendly 502, got %d", code)
	}
	code, out = ca.do("POST", "/api/add", map[string]any{"pub": out["pub"], "name": "Bob", "addr": addr, "label": "Bobby"}, nil)
	if code != 200 {
		t.Fatalf("add: %d %v", code, out)
	}
	_, st := cb.do("GET", "/api/state", nil, nil)
	reqs := st["requests"].([]any)
	if len(reqs) != 1 || reqs[0].(map[string]any)["their_name"] != "Alex" {
		t.Fatalf("bob should see one request from Alex: %v", reqs)
	}
	pub := reqs[0].(map[string]any)["pub"]
	if code, _ := cb.do("POST", "/api/send", map[string]any{"c": pub, "body": "hi"}, nil); code != 400 {
		t.Fatalf("messaging an unaccepted contact must be refused, got %d", code)
	}
	if code, _ := cb.do("POST", "/api/contact", map[string]any{"c": pub, "action": "accept"}, nil); code != 200 {
		t.Fatal("accept failed")
	}
	_, st = cb.do("GET", "/api/state", nil, nil)
	if len(st["requests"].([]any)) != 0 || len(st["contacts"].([]any)) != 1 {
		t.Fatalf("after accept: %v", st)
	}
}

func TestSettingsValidation(t *testing.T) {
	n := mk(t, "Alex")
	c, _ := ui(t, n)
	if code, _ := c.do("POST", "/api/settings", map[string]any{"name": "Al", "retention_days": -5, "secret_detection": true}, nil); code != 400 {
		t.Fatalf("out-of-range retention must be refused, got %d", code)
	}
	if code, _ := c.do("POST", "/api/settings", map[string]any{"name": "Al", "retention_days": 30, "secret_detection": true}, nil); code != 200 {
		t.Fatal("valid settings refused")
	}
	if n.ID.Name != "Al" || n.Cfg.DefaultRetentionDays != 30 {
		t.Fatal("settings not applied")
	}
}

func TestSettingsWhileChatting(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	pub := befriend(t, a, b)
	c, _ := ui(t, a)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			if code, out := c.do("POST", "/api/settings", map[string]any{"name": fmt.Sprintf("Alex %d", i), "retention_days": i, "secret_detection": true}, nil); code != 200 {
				t.Errorf("settings: %d %v", code, out)
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			if code, out := c.do("POST", "/api/send", map[string]any{"c": pub, "body": fmt.Sprintf("hello %d", i)}, nil); code != 200 || out["state"] != "delivered" {
				t.Errorf("send: %d %v", code, out)
			}
			if code, out := c.do("GET", "/api/state", nil, nil); code != 200 {
				t.Errorf("state: %d %v", code, out)
			}
		}
	}()
	wg.Wait()
}

func TestConcurrentBurnRevealOnce(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	pub := befriend(t, a, b)
	m, err := a.Send(mustContact(t, a, pub), "one-time note", node.SendOpts{Burn: true})
	if err != nil {
		t.Fatal(err)
	}
	cb, _ := ui(t, b)
	var wg sync.WaitGroup
	var successes atomic.Int32
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, out := cb.do("POST", "/api/reveal", map[string]any{"c": identity.Encode(a.ID.Pub), "id": m.Env.ID}, nil)
			if code == 200 && out["body"] == "one-time note" {
				successes.Add(1)
			} else if code != 404 {
				t.Errorf("reveal: %d %v", code, out)
			}
		}()
	}
	wg.Wait()
	if got := successes.Load(); got != 1 {
		t.Fatalf("read-once body revealed %d times", got)
	}
}

func TestContactBookEditPreservesBroadcastInformation(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	pub := befriend(t, a, b)
	ca, _ := ui(t, a)
	a.OnPeer(pub, "Robert online", fmt.Sprintf("127.0.0.1:%d", b.Port()), "Bob laptop", "linux/amd64")
	code, out := ca.do("POST", "/api/contact", map[string]any{"c": pub, "action": "edit", "label": "Bobby", "notes": "Team contact", "address": fmt.Sprintf("127.0.0.1:%d", b.Port())}, nil)
	if code != 200 {
		t.Fatalf("edit: %d %v", code, out)
	}
	_, state := ca.do("GET", "/api/state", nil, nil)
	c := state["contacts"].([]any)[0].(map[string]any)
	if c["label"] != "Bobby" || c["their_name"] != "Robert online" || c["device"] != "Bob laptop" || c["platform"] != "linux/amd64" || c["notes"] != "Team contact" {
		t.Fatalf("contact fields %+v", c)
	}
	me := state["me"].(map[string]any)
	if me["pub"] == "" || me["device"] == "" || me["platform"] == "" {
		t.Fatalf("self profile %+v", me)
	}
	if code, _ := ca.do("POST", "/api/contact", map[string]any{"c": pub, "action": "edit", "label": "Bobby", "address": "127.0.0.1:99999"}, nil); code != 400 {
		t.Fatalf("bad port accepted: %d", code)
	}
}

func TestNetworkViewIncludesPeersAndLocalInterfaces(t *testing.T) {
	a := mk(t, "Alex")
	ca, _ := ui(t, a)
	b := mk(t, "Bob")
	a.OnPeer(identity.Encode(b.ID.Pub), "Bob", fmt.Sprintf("127.0.0.1:%d", b.Port()), "Bob PC", "linux/amd64")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, out := ca.do("GET", "/api/network", nil, nil)
			if code != 200 {
				t.Errorf("network %d %v", code, out)
				return
			}
			if _, ok := out["interfaces"].([]any); !ok {
				t.Errorf("missing interfaces: %+v", out)
			}
			found := false
			for _, v := range out["devices"].([]any) {
				d := v.(map[string]any)
				if d["pub"] == identity.Encode(b.ID.Pub) && d["broadcast_name"] == "Bob" {
					found = true
				}
			}
			if !found {
				t.Error("peer absent from network inventory")
			}
		}()
	}
	wg.Wait()
}

func TestFileUploadAndAuthenticatedDownload(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	pub := befriend(t, a, b)
	ca, _ := ui(t, a)
	cb, _ := ui(t, b)
	payload := bytes.Repeat([]byte{0, 1, 255, 128, 17}, 40000)
	req, _ := http.NewRequest("POST", ca.ts.URL+"/api/files?c="+pub+"&name=report.bin", bytes.NewReader(payload))
	req.Header.Set("X-SpearHead", "1")
	req.Header.Set("Content-Type", "application/octet-stream")
	response, err := ca.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var out map[string]any
	json.NewDecoder(response.Body).Decode(&out)
	if response.StatusCode != 202 {
		t.Fatalf("upload %d %v", response.StatusCode, out)
	}
	id := out["id"].(string)
	peer := identity.Encode(a.ID.Pub)
	until := time.Now().Add(5 * time.Second)
	for b.St.GetMessage(peer, id) == nil && time.Now().Before(until) {
		time.Sleep(10 * time.Millisecond)
	}
	url := cb.ts.URL + "/api/file?c=" + peer + "&id=" + id
	response, err = cb.c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("download %d, %d bytes, %v", response.StatusCode, len(got), err)
	}
	if !strings.Contains(response.Header.Get("Content-Disposition"), "report.bin") {
		t.Fatal("download filename missing")
	}
	anon, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	anon.Body.Close()
	if anon.StatusCode != 401 {
		t.Fatalf("anonymous download %d", anon.StatusCode)
	}
}

func TestMessageDeleteIsAuthenticatedLocalAndScopedToContact(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	pub := befriend(t, a, b)
	ca, _ := ui(t, a)
	c, _ := a.St.GetContact(pub)
	m, err := a.Send(c, "Delete only my copy", node.SendOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := ca.do("POST", "/api/message/delete", map[string]any{"c": pub, "id": m.Env.ID}, map[string]string{"X-SpearHead": ""}); code != 403 {
		t.Fatalf("unauthorized delete %d", code)
	}
	if a.St.GetMessage(pub, m.Env.ID) == nil {
		t.Fatal("unauthorized request removed message")
	}
	if code, _ := ca.do("POST", "/api/message/delete", map[string]any{"c": identity.Encode(a.ID.Pub), "id": m.Env.ID}, nil); code != 404 {
		t.Fatalf("wrong contact %d", code)
	}
	if code, _ := ca.do("POST", "/api/message/delete", map[string]any{"c": pub, "id": "../../identity.json"}, nil); code != 400 {
		t.Fatalf("invalid id %d", code)
	}
	if code, out := ca.do("POST", "/api/message/delete", map[string]any{"c": pub, "id": m.Env.ID}, nil); code != 200 || out["scope"] != "local" {
		t.Fatalf("delete %d %+v", code, out)
	}
	if a.St.GetMessage(pub, m.Env.ID) != nil {
		t.Fatal("local content remains")
	}
	if b.St.GetMessage(identity.Encode(a.ID.Pub), m.Env.ID) == nil {
		t.Fatal("recipient's copy removed")
	}
}
