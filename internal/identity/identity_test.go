package identity

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWordlist(t *testing.T) {
	if len(wordlist) != 256 {
		t.Fatalf("want 256 words, got %d", len(wordlist))
	}
	seen := map[string]bool{}
	for _, w := range wordlist {
		if w == "" || seen[w] {
			t.Fatalf("empty or duplicate word %q", w)
		}
		seen[w] = true
	}
}

func TestCodes(t *testing.T) {
	a, _ := Create(t.TempDir(), "a")
	b, _ := Create(t.TempDir(), "b")
	if SafetyCode(a.Pub, b.Pub) != SafetyCode(b.Pub, a.Pub) {
		t.Fatal("safety code must be symmetric")
	}
	c, _ := Create(t.TempDir(), "c")
	if SafetyCode(a.Pub, b.Pub) == SafetyCode(a.Pub, c.Pub) {
		t.Fatal("different pairs should differ")
	}
	if Fingerprint(a.Pub) != Fingerprint(a.Pub) || len(Fingerprint(a.Pub)) < 7 {
		t.Fatal("fingerprint unstable")
	}
}

func TestPersistence(t *testing.T) {
	home := t.TempDir()
	a, _ := Create(home, "Alex\x07")
	if a.Name != "Alex" {
		t.Fatalf("control chars not stripped: %q", a.Name)
	}
	b, err := Load(home)
	if err != nil || string(a.Pub) != string(b.Pub) {
		t.Fatal("reload mismatch", err)
	}
	if _, err := Create(home, "again"); err == nil {
		t.Fatal("must not overwrite identity")
	}
}

func TestWindowsIdentityUsesPlatformProtection(t *testing.T) {
	if !keyProtectionAvailable() {
		t.Skip("platform key protection is not available")
	}
	home := t.TempDir()
	if _, err := Create(home, "Alex"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(home, "identity.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"seed"`) || !strings.Contains(string(b), `"protected"`) {
		t.Fatalf("identity was not DPAPI-protected: %s", b)
	}
}
