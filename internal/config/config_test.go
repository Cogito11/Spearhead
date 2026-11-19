package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHomeRenamePreservesExistingIdentity(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("AppData", root)
	t.Setenv("SPEARHEAD_HOME", "")
	t.Setenv("RACKET_HOME", "")
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	home, legacy := filepath.Join(dir, "spearhead"), filepath.Join(dir, "racket")
	if got := Home(""); got != home {
		t.Fatalf("fresh home: %q", got)
	}
	for _, path := range []string{legacy, home} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "identity.json"), []byte("{}"), 0600); err != nil {
			t.Fatal(err)
		}
		if got := Home(""); got != path {
			t.Fatalf("existing identity home: %q, want %q", got, path)
		}
	}
	t.Setenv("RACKET_HOME", "legacy-env")
	if got := Home(""); got != "legacy-env" {
		t.Fatal(got)
	}
	t.Setenv("SPEARHEAD_HOME", "new-env")
	if got := Home(""); got != "new-env" {
		t.Fatal(got)
	}
	if got := Home("explicit"); got != "explicit" {
		t.Fatal(got)
	}
}
