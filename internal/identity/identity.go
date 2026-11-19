// Package identity manages a node's Ed25519 identity and the human-readable
// fingerprint and safety-code renderings of public keys.
package identity

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type Identity struct {
	mu   sync.RWMutex
	Priv ed25519.PrivateKey
	Pub  ed25519.PublicKey
	Name string
}

type keyFile struct {
	// Seed is retained only to migrate identities created by versions before
	// platform key protection. New Windows installs use Protected instead.
	Seed      string `json:"seed,omitempty"`
	Protected string `json:"protected,omitempty"`
	Name      string `json:"name"`
}

func path(home string) string { return filepath.Join(home, "identity.json") }

// Exists reports whether an identity has been created in home.
func Exists(home string) bool { _, err := os.Stat(path(home)); return err == nil }

// Create generates a new identity. It refuses to overwrite an existing one.
func Create(home, name string) (*Identity, error) {
	if Exists(home) {
		return nil, errors.New("an identity already exists in " + home)
	}
	if err := os.MkdirAll(home, 0o700); err != nil {
		return nil, err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	id := &Identity{Priv: priv, Pub: pub, Name: CleanName(name)}
	return id, id.save(home)
}

// Load reads the identity from home.
func Load(home string) (*Identity, error) {
	b, err := os.ReadFile(path(home))
	if err != nil {
		return nil, errors.New("SpearHead is not set up yet. Run `spearhead` with no arguments to get started")
	}
	var kf keyFile
	if err := json.Unmarshal(b, &kf); err != nil {
		return nil, err
	}
	var seed []byte
	if kf.Protected != "" {
		sealed, err := base64.RawURLEncoding.DecodeString(kf.Protected)
		if err != nil {
			return nil, errors.New("corrupt protected identity file")
		}
		seed, err = unprotectSeed(sealed)
		if err != nil {
			return nil, errors.New("could not unlock this identity on this device: " + err.Error())
		}
	} else {
		seed, err = base64.RawURLEncoding.DecodeString(kf.Seed)
	}
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, errors.New("corrupt identity file")
	}
	priv := ed25519.NewKeyFromSeed(seed)
	for i := range seed {
		seed[i] = 0
	}
	id := &Identity{Priv: priv, Pub: priv.Public().(ed25519.PublicKey), Name: kf.Name}
	if kf.Protected == "" && keyProtectionAvailable() {
		if err := id.save(home); err != nil {
			return nil, errors.New("could not secure the existing identity: " + err.Error())
		}
	}
	return id, nil
}

func (i *Identity) save(home string) error {
	kf := keyFile{Name: i.Name}
	if keyProtectionAvailable() {
		sealed, err := protectSeed(i.Priv.Seed())
		if err != nil {
			return err
		}
		kf.Protected = base64.RawURLEncoding.EncodeToString(sealed)
	} else {
		kf.Seed = base64.RawURLEncoding.EncodeToString(i.Priv.Seed())
	}
	b, _ := json.MarshalIndent(kf, "", "  ")
	return os.WriteFile(path(home), b, 0o600)
}

// DisplayName returns the current name safely during discovery and handshakes.
func (i *Identity) DisplayName() string {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.Name
}

// SetName changes and saves the display name.
func (i *Identity) SetName(home, name string) error {
	i.mu.Lock()
	defer i.mu.Unlock()
	old := i.Name
	i.Name = CleanName(name)
	if err := i.save(home); err != nil {
		i.Name = old
		return err
	}
	return nil
}

// CleanName strips control characters and limits length. Display names are
// only ever suggestions from the other party.
func CleanName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 48 {
		s = string(r[:48])
	}
	return s
}

// Encode renders a public key as unpadded base64url.
func Encode(pub []byte) string { return base64.RawURLEncoding.EncodeToString(pub) }

// Decode parses a public key produced by Encode.
func Decode(s string) (ed25519.PublicKey, error) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil, errors.New("invalid public key")
	}
	return ed25519.PublicKey(b), nil
}

func words(b []byte) []string {
	out := make([]string, len(b))
	for i, x := range b {
		out[i] = wordlist[x]
	}
	return out
}

// Fingerprint is a 4-word (32-bit) display name for a key. It is for
// recognition only and must not be used for verification.
func Fingerprint(pub []byte) string {
	h := sha256.Sum256(pub)
	return strings.Join(words(h[:4]), "-")
}

// SafetyCode is an 8-word (64-bit) code derived from both keys; it is the same
// on both sides and is compared out-of-band to verify a contact.
func SafetyCode(a, b []byte) string {
	lo, hi := a, b
	if bytes.Compare(a, b) > 0 {
		lo, hi = b, a
	}
	h := sha256.New()
	h.Write([]byte("racket/v1/safety"))
	h.Write(lo)
	h.Write(hi)
	w := words(h.Sum(nil)[:8])
	return strings.Join(w[:4], " ") + "\n" + strings.Join(w[4:], " ")
}
