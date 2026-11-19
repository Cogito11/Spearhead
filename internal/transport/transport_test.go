package transport

import (
	"bytes"
	"errors"
	"net"
	"testing"
	"time"

	"spearhead/internal/identity"
)

func pair(t *testing.T, expect []byte) (*Conn, *Conn, error) {
	a, _ := identity.Create(t.TempDir(), "alice")
	b, _ := identity.Create(t.TempDir(), "bob")
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	type res struct {
		c   *Conn
		err error
	}
	ch := make(chan res, 1)
	go func() {
		raw, _ := ln.Accept()
		c, err := Accept(raw, b, 0)
		ch <- res{c, err}
	}()
	if expect == nil {
		expect = b.Pub
	}
	c, err := Dial(ln.Addr().String(), a, 0, expect, time.Second)
	r := <-ch
	if err != nil {
		return nil, nil, err
	}
	if !bytes.Equal(c.Peer, b.Pub) || !bytes.Equal(r.c.Peer, a.Pub) || r.c.PeerName != "alice" {
		t.Fatal("peers not authenticated correctly")
	}
	return c, r.c, nil
}

func TestFramesRoundTrip(t *testing.T) {
	c, s, err := pair(t, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		msg := bytes.Repeat([]byte{byte(i)}, 1000*i+1)
		go c.WriteFrame(msg)
		got, err := s.ReadFrame()
		if err != nil || !bytes.Equal(got, msg) {
			t.Fatalf("frame %d mismatch: %v", i, err)
		}
	}
}

func TestKeyMismatch(t *testing.T) {
	other, _ := identity.Create(t.TempDir(), "x")
	_, _, err := pair(t, other.Pub)
	if !errors.Is(err, ErrKeyMismatch) {
		t.Fatalf("want ErrKeyMismatch, got %v", err)
	}
}

func TestHKDFKnownAnswer(t *testing.T) {
	// RFC 5869 test case 1 (first 32 bytes of OKM).
	ikm := bytes.Repeat([]byte{0x0b}, 22)
	salt := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12}
	info := []byte{0xf0, 0xf1, 0xf2, 0xf3, 0xf4, 0xf5, 0xf6, 0xf7, 0xf8, 0xf9}
	got := hkdfExpand(hkdfExtract(salt, ikm), info, 32)
	want := []byte{0x3c, 0xb2, 0x5f, 0x25, 0xfa, 0xac, 0xd5, 0x7a, 0x90, 0x43, 0x4f, 0x64, 0xd0, 0x36, 0x2f, 0x2a,
		0x2d, 0x2d, 0x0a, 0x90, 0xcf, 0x1a, 0x5a, 0x4c, 0x5d, 0xb0, 0x2d, 0x56, 0xec, 0xc4, 0xc5, 0xbf}
	if !bytes.Equal(got, want) {
		t.Fatalf("HKDF mismatch: %x", got)
	}
}
