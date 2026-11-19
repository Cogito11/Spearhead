// Package transport implements the authenticated, encrypted peer channel.
//
// NOTE: this handshake (ephemeral X25519, transcript-bound Ed25519 signatures,
// HKDF-SHA256, AES-256-GCM) is built from standard-library primitives as a
// custom protocol. It requires independent review before production use.
// See docs/ARCHITECTURE.md for the implemented protocol and storage boundaries.
package transport

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"spearhead/internal/identity"
)

const (
	magic            = "RKT1"
	MaxFrame         = 1 << 20
	handshakeTimeout = 10 * time.Second
)

// ErrKeyMismatch is returned when a dialed peer proves a different identity
// than the one expected.
var ErrKeyMismatch = errors.New("KEY MISMATCH: peer proved a different identity than expected")

type Conn struct {
	raw        net.Conn
	send, recv cipher.AEAD
	sctr, rctr uint64
	wmu, rmu   sync.Mutex

	Peer         ed25519.PublicKey
	PeerDevice   string
	PeerPlatform string
	PeerName     string
	PeerPort     int // listen port the peer advertises (0 if none)
}

type authMsg struct {
	Pub      string `json:"pub"`
	Name     string `json:"name"`
	Port     int    `json:"port,omitempty"`
	Sig      string `json:"sig"`
	Device   string `json:"device,omitempty"`
	Platform string `json:"platform,omitempty"`
}

func hkdfExtract(salt, ikm []byte) []byte {
	m := hmac.New(sha256.New, salt)
	m.Write(ikm)
	return m.Sum(nil)
}

// hkdfExpand implements a single-block HKDF-Expand (n <= 32).
func hkdfExpand(prk, info []byte, n int) []byte {
	m := hmac.New(sha256.New, prk)
	m.Write(info)
	m.Write([]byte{1})
	return m.Sum(nil)[:n]
}

func newAEAD(key []byte) cipher.AEAD {
	b, _ := aes.NewCipher(key)
	g, _ := cipher.NewGCM(b)
	return g
}

func authInput(role byte, transcript []byte) []byte {
	return append(append([]byte("racket/v1/auth"), role), transcript...)
}

// Dial connects to addr and authenticates. If expect is non-nil, the peer must
// prove exactly that identity or ErrKeyMismatch is returned.
func Dial(addr string, id *identity.Identity, port int, expect ed25519.PublicKey, timeout time.Duration) (*Conn, error) {
	raw, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return nil, err
	}
	c, err := handshake(raw, id, port, true, expect)
	if err != nil {
		raw.Close()
		return nil, err
	}
	return c, nil
}

// Accept performs the responder side of the handshake on an inbound connection.
func Accept(raw net.Conn, id *identity.Identity, port int) (*Conn, error) {
	return handshake(raw, id, port, false, nil)
}

func handshake(raw net.Conn, id *identity.Identity, port int, initiator bool, expect ed25519.PublicKey) (*Conn, error) {
	raw.SetDeadline(time.Now().Add(handshakeTimeout))
	defer raw.SetDeadline(time.Time{})

	curve := ecdh.X25519()
	eph, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	var ephI, ephR []byte
	if initiator {
		if _, err := raw.Write(append([]byte(magic), eph.PublicKey().Bytes()...)); err != nil {
			return nil, err
		}
		ephR = make([]byte, 32)
		if _, err := io.ReadFull(raw, ephR); err != nil {
			return nil, err
		}
		ephI = eph.PublicKey().Bytes()
	} else {
		hello := make([]byte, 4+32)
		if _, err := io.ReadFull(raw, hello); err != nil {
			return nil, err
		}
		if string(hello[:4]) != magic {
			return nil, errors.New("not a spearhead peer (bad magic)")
		}
		ephI = hello[4:]
		ephR = eph.PublicKey().Bytes()
		if _, err := raw.Write(ephR); err != nil {
			return nil, err
		}
	}
	peerEph, err := curve.NewPublicKey(map[bool][]byte{true: ephR, false: ephI}[initiator])
	if err != nil {
		return nil, err
	}
	shared, err := eph.ECDH(peerEph)
	if err != nil {
		return nil, err
	}
	th := sha256.New()
	th.Write([]byte("racket/v1/transcript"))
	th.Write(ephI)
	th.Write(ephR)
	transcript := th.Sum(nil)
	prk := hkdfExtract(transcript, shared)
	kI2R := newAEAD(hkdfExpand(prk, []byte("racket/v1/c2s"), 32))
	kR2I := newAEAD(hkdfExpand(prk, []byte("racket/v1/s2c"), 32))

	c := &Conn{raw: raw}
	role, peerRole := byte('S'), byte('C')
	c.send, c.recv = kR2I, kI2R
	if initiator {
		role, peerRole = 'C', 'S'
		c.send, c.recv = kI2R, kR2I
	}

	device, platform := identity.LocalDevice()
	out, _ := json.Marshal(authMsg{
		Device: device, Platform: platform,
		Pub:  identity.Encode(id.Pub),
		Name: id.DisplayName(),
		Port: port,
		Sig:  base64.RawURLEncoding.EncodeToString(ed25519.Sign(id.Priv, authInput(role, transcript))),
	})
	if err := c.writeFrame(out); err != nil {
		return nil, err
	}
	in, err := c.ReadFrame()
	if err != nil {
		return nil, err
	}
	var am authMsg
	if err := json.Unmarshal(in, &am); err != nil {
		return nil, errors.New("bad auth frame")
	}
	pub, err := identity.Decode(am.Pub)
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(am.Sig)
	if err != nil || !ed25519.Verify(pub, authInput(peerRole, transcript), sig) {
		return nil, errors.New("peer failed to prove its identity")
	}
	if expect != nil && !bytes.Equal(expect, pub) {
		return nil, fmt.Errorf("%w (got %s)", ErrKeyMismatch, identity.Fingerprint(pub))
	}
	c.Peer, c.PeerName = pub, identity.CleanName(am.Name)
	c.PeerDevice, c.PeerPlatform = identity.CleanName(am.Device), identity.CleanName(am.Platform)
	if am.Port > 0 && am.Port < 65536 {
		c.PeerPort = am.Port
	}
	return c, nil
}

func nonce(ctr uint64) []byte {
	n := make([]byte, 12)
	binary.BigEndian.PutUint64(n[4:], ctr)
	return n
}

func (c *Conn) writeFrame(p []byte) error {
	if len(p) > MaxFrame {
		return errors.New("frame too large")
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	ct := c.send.Seal(nil, nonce(c.sctr), p, nil)
	c.sctr++
	hdr := make([]byte, 4, 4+len(ct))
	binary.BigEndian.PutUint32(hdr, uint32(len(ct)))
	_, err := c.raw.Write(append(hdr, ct...))
	return err
}

// WriteFrame encrypts and sends one frame.
func (c *Conn) WriteFrame(p []byte) error { return c.writeFrame(p) }

// ReadFrame receives and decrypts one frame.
func (c *Conn) ReadFrame() ([]byte, error) {
	c.rmu.Lock()
	defer c.rmu.Unlock()
	var hdr [4]byte
	if _, err := io.ReadFull(c.raw, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n < 16 || n > MaxFrame+16 {
		return nil, errors.New("bad frame length")
	}
	ct := make([]byte, n)
	if _, err := io.ReadFull(c.raw, ct); err != nil {
		return nil, err
	}
	pt, err := c.recv.Open(nil, nonce(c.rctr), ct, nil)
	if err != nil {
		return nil, errors.New("frame authentication failed")
	}
	c.rctr++
	return pt, nil
}

func (c *Conn) SetDeadline(t time.Time) error { return c.raw.SetDeadline(t) }
func (c *Conn) Close() error                  { return c.raw.Close() }
func (c *Conn) RemoteAddr() net.Addr          { return c.raw.RemoteAddr() }
