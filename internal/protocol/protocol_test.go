package protocol

import (
	"testing"

	"spearhead/internal/identity"
)

func TestSignVerifyTamper(t *testing.T) {
	a, _ := identity.Create(t.TempDir(), "a")
	b, _ := identity.Create(t.TempDir(), "b")
	e, err := New(a, b.Pub, KindChat, "hello", 60, false, "")
	if err != nil || e.Verify() != nil {
		t.Fatal("fresh envelope must verify", err)
	}
	mut := map[string]func(x *Envelope){
		"body": func(x *Envelope) { x.Body = "hellO" },
		"ts":   func(x *Envelope) { x.TS++ },
		"kind": func(x *Envelope) { x.Kind = KindRequest },
		"ttl":  func(x *Envelope) { x.TTL++ },
		"burn": func(x *Envelope) { x.Burn = true },
		"to":   func(x *Envelope) { x.To = []string{identity.Encode(a.Pub)} },
		"prev": func(x *Envelope) { x.Prev = "00000000000000000000000000000000000000000000000000000000000000ff" },
		"id":   func(x *Envelope) { x.ID = "ffffffffffffffffffffffffffffffff" },
	}
	for name, f := range mut {
		c := *e
		f(&c)
		if c.Verify() == nil {
			t.Errorf("tampering with %s not detected", name)
		}
	}
	if e.Hash() != e.Hash() || len(e.Hash()) != 64 {
		t.Fatal("hash unstable")
	}
}

func TestFormatRules(t *testing.T) {
	a, _ := identity.Create(t.TempDir(), "a")
	if _, err := New(a, a.Pub, "bogus", "x", 0, false, ""); err == nil {
		t.Fatal("unknown kind accepted")
	}
	if _, err := New(a, a.Pub, KindChat, "x", 0, false, "not-a-hash"); err == nil {
		t.Fatal("bad prev accepted")
	}
}
