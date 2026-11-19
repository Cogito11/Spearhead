// Package protocol defines the signed message envelope.
package protocol

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"spearhead/internal/identity"
)

const (
	KindChat    = "chat"
	KindRequest = "request"

	MaxBody = 256 * 1024
)

var (
	idRe   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	hashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type Envelope struct {
	ID   string   `json:"id"`
	From string   `json:"from"`
	To   []string `json:"to"`
	TS   int64    `json:"ts"`
	Kind string   `json:"kind"`
	TTL  int64    `json:"ttl"`
	Burn bool     `json:"burn"`
	Prev string   `json:"prev"`
	Body string   `json:"body"`
	Sig  string   `json:"sig"`
}

func newID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// New builds and signs an envelope from id to a single recipient.
func New(id *identity.Identity, to ed25519.PublicKey, kind, body string, ttl int64, burn bool, prev string) (*Envelope, error) {
	e := &Envelope{
		ID: newID(), From: identity.Encode(id.Pub), To: []string{identity.Encode(to)},
		TS: time.Now().Unix(), Kind: kind, TTL: ttl, Burn: burn, Prev: prev, Body: body,
	}
	if err := e.checkFormat(); err != nil {
		return nil, err
	}
	e.Sig = base64.RawURLEncoding.EncodeToString(ed25519.Sign(id.Priv, e.signingInput()))
	return e, nil
}

func (e *Envelope) signingInput() []byte {
	bh := sha256.Sum256([]byte(e.Body))
	return []byte(fmt.Sprintf("racket/v1/msg\n%s\n%s\n%s\n%d\n%s\n%d\n%t\n%s\n%x",
		e.ID, e.From, strings.Join(e.To, ","), e.TS, e.Kind, e.TTL, e.Burn, e.Prev, bh))
}

func (e *Envelope) checkFormat() error {
	switch {
	case !idRe.MatchString(e.ID):
		return errors.New("bad id")
	case e.Kind != KindChat && e.Kind != KindRequest && e.Kind != KindFile:
		return errors.New("unknown kind")
	case e.Prev != "" && !hashRe.MatchString(e.Prev):
		return errors.New("bad prev")
	case len(e.Body) > MaxBody:
		return errors.New("body too large")
	case len(e.To) == 0 || len(e.To) > 64:
		return errors.New("bad recipient list")
	case e.TTL < 0:
		return errors.New("bad ttl")
	}
	if e.Kind == KindFile {
		if e.Burn {
			return errors.New("files cannot be read-once")
		}
		if _, err := ParseFile(e.Body); err != nil {
			return err
		}
	}
	if _, err := identity.Decode(e.From); err != nil {
		return errors.New("bad sender key")
	}
	for _, t := range e.To {
		if _, err := identity.Decode(t); err != nil {
			return errors.New("bad recipient key")
		}
	}
	return nil
}

// Verify checks field formats and the signature.
func (e *Envelope) Verify() error {
	if err := e.checkFormat(); err != nil {
		return err
	}
	pub, _ := identity.Decode(e.From)
	sig, err := base64.RawURLEncoding.DecodeString(e.Sig)
	if err != nil || !ed25519.Verify(pub, e.signingInput(), sig) {
		return errors.New("bad signature")
	}
	return nil
}

// Hash identifies this message in a sender's hash chain.
func (e *Envelope) Hash() string {
	sig, _ := base64.RawURLEncoding.DecodeString(e.Sig)
	h := sha256.Sum256(append(e.signingInput(), sig...))
	return hex.EncodeToString(h[:])
}

// AddressedTo reports whether pub is a recipient.
func (e *Envelope) AddressedTo(pub ed25519.PublicKey) bool {
	me := identity.Encode(pub)
	for _, t := range e.To {
		if t == me {
			return true
		}
	}
	return false
}
