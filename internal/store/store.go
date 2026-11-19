// Package store persists contacts, messages, the outbox, and seen peers as
// small JSON files written atomically. See docs/ARCHITECTURE.md.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"spearhead/internal/identity"
	"spearhead/internal/protocol"
)

const (
	StatusPending  = "pending"
	StatusAccepted = "accepted"
	StatusBlocked  = "blocked"

	DirIn  = "in"
	DirOut = "out"

	StateQueued    = "queued"
	StateDelivered = "delivered"
	StateFailed    = "failed"
	StateUnread    = "unread"
	StateRead      = "read"
)

type Contact struct {
	Pub       string `json:"pub"`
	Label     string `json:"label"`
	TheirName string `json:"their_name"`
	Status    string `json:"status"`
	Verified  bool   `json:"verified"`
	FirstSeen int64  `json:"first_seen"`
	LastSeen  int64  `json:"last_seen"`
	LastAddr  string `json:"last_addr"`
	Notes     string `json:"notes,omitempty"`
	Device    string `json:"device,omitempty"`
	Platform  string `json:"platform,omitempty"`
}

type Message struct {
	Env     protocol.Envelope `json:"env"`
	Dir     string            `json:"dir"`
	State   string            `json:"state"`
	Reason  string            `json:"reason,omitempty"`
	Hash    string            `json:"hash"`
	Stored  int64             `json:"stored"`
	Expires int64             `json:"expires,omitempty"`
	Gap     bool              `json:"gap,omitempty"`
	Thread  string            `json:"thread"`
	path    string
}

type OutboxEntry struct {
	Thread   string `json:"thread"`
	ID       string `json:"id"`
	Attempts int    `json:"attempts"`
	NextTry  int64  `json:"next_try"`
}

type Peer struct {
	Pub      string `json:"pub"`
	Name     string `json:"name"`
	Addr     string `json:"addr"`
	Seen     int64  `json:"seen"`
	Device   string `json:"device,omitempty"`
	Platform string `json:"platform,omitempty"`
}

type Store struct {
	root string
	mu   sync.Mutex
}

func Open(home string) (*Store, error) {
	for _, d := range []string{"contacts", "threads", "outbox", "peers", "files"} {
		if err := os.MkdirAll(filepath.Join(home, d), 0o700); err != nil {
			return nil, err
		}
	}
	return &Store{root: home}, nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (s *Store) p(parts ...string) string {
	return filepath.Join(append([]string{s.root}, parts...)...)
}

// ---- contacts ----

func (s *Store) SaveContact(c *Contact) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return writeJSON(s.p("contacts", c.Pub+".json"), c)
}

// UpdateContact changes a fresh contact under the store lock, preserving edits
// made by the UI while a handshake or discovery callback was in flight.
func (s *Store) UpdateContact(pub string, update func(*Contact)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var c Contact
	path := s.p("contacts", pub+".json")
	if err := readJSON(path, &c); err != nil {
		return err
	}
	update(&c)
	return writeJSON(path, &c)
}

func (s *Store) GetContact(pub string) (*Contact, error) {
	var c Contact
	if err := readJSON(s.p("contacts", pub+".json"), &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *Store) DeleteContact(pub string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	os.RemoveAll(s.p("threads", pub))
	os.RemoveAll(s.p("files", pub))
	return os.Remove(s.p("contacts", pub+".json"))
}

func (s *Store) ListContacts() []*Contact {
	files, _ := filepath.Glob(s.p("contacts", "*.json"))
	var out []*Contact
	for _, f := range files {
		var c Contact
		if readJSON(f, &c) == nil {
			out = append(out, &c)
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Label) < strings.ToLower(out[j].Label) })
	return out
}

// FindContact resolves a label, fingerprint prefix, or key prefix to one contact.
func (s *Store) FindContact(q string) (*Contact, error) {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil, errors.New("no contact given")
	}
	var exact, partial []*Contact
	for _, c := range s.ListContacts() {
		pub, err := identity.Decode(c.Pub)
		if err != nil {
			continue
		}
		fp := identity.Fingerprint(pub)
		switch {
		case strings.ToLower(c.Label) == q || fp == q:
			exact = append(exact, c)
		case strings.HasPrefix(strings.ToLower(c.Label), q) || (len(q) >= 3 && strings.HasPrefix(fp, q)) ||
			(len(q) >= 6 && strings.HasPrefix(strings.ToLower(c.Pub), q)):
			partial = append(partial, c)
		}
	}
	pick := exact
	if len(pick) == 0 {
		pick = partial
	}
	switch len(pick) {
	case 0:
		return nil, fmt.Errorf("no contact matches %q", q)
	case 1:
		return pick[0], nil
	}
	return nil, fmt.Errorf("%q is ambiguous (%d matches); be more specific", q, len(pick))
}

// ---- messages ----

func (s *Store) threadDir(thread string) string { return s.p("threads", thread) }

// AddMessage stores m. It returns false if a message with the same id exists.
func (s *Store) AddMessage(m *Message) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.IsDeleted(m.Thread, m.Env.ID) {
		return false, nil
	}
	dir := s.threadDir(m.Thread)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, err
	}
	if dup, _ := filepath.Glob(filepath.Join(dir, "*-"+m.Env.ID+".json")); len(dup) > 0 {
		return false, nil
	}
	m.Stored = time.Now().UnixNano()
	m.path = filepath.Join(dir, fmt.Sprintf("%020d-%s.json", m.Stored, m.Env.ID))
	return true, writeJSON(m.path, m)
}

func (s *Store) SaveMessage(m *Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.IsDeleted(m.Thread, m.Env.ID) {
		return nil
	}
	if m.path == "" {
		return errors.New("message has no path")
	}
	return writeJSON(m.path, m)
}

func (s *Store) DeleteMessage(m *Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	os.Remove(s.FilePath(m.Thread, m.Env.ID))
	return os.Remove(m.path)
}

func (s *Store) Thread(thread string) []*Message {
	files, _ := filepath.Glob(filepath.Join(s.threadDir(thread), "*.json"))
	sort.Strings(files)
	var out []*Message
	for _, f := range files {
		var m Message
		if readJSON(f, &m) == nil && !s.IsDeleted(thread, m.Env.ID) {
			m.path = f
			out = append(out, &m)
		}
	}
	return out
}

func (s *Store) GetMessage(thread, id string) *Message {
	for _, m := range s.Thread(thread) {
		if m.Env.ID == id {
			return m
		}
	}
	return nil
}

// LastOutHash returns the hash of our most recent non-burn message in thread.
func (s *Store) LastOutHash(thread string) string {
	var stored int64
	var hash string
	for _, m := range s.Thread(thread) {
		if m.Dir == DirOut && !m.Env.Burn && m.Stored > stored {
			stored, hash = m.Stored, m.Hash
		}
	}
	for _, receipt := range s.deletionReceipts(thread) {
		if receipt.Dir == DirOut && receipt.Hash != "" && receipt.Stored > stored {
			stored, hash = receipt.Stored, receipt.Hash
		}
	}
	return hash
}

// HasIncomingHash includes body-free deletion receipts so an intentional local
// deletion does not make the next incoming message appear to have a gap.
func (s *Store) HasIncomingHash(thread, hash string) bool {
	for _, m := range s.Thread(thread) {
		if m.Dir == DirIn && m.Hash == hash {
			return true
		}
	}
	for _, receipt := range s.deletionReceipts(thread) {
		if receipt.Dir == DirIn && receipt.Hash != "" && receipt.Hash == hash {
			return true
		}
	}
	return false
}

// Unread returns unread incoming messages from accepted contacts, oldest first.
func (s *Store) Unread() []*Message {
	var out []*Message
	for _, c := range s.ListContacts() {
		if c.Status != StatusAccepted {
			continue
		}
		for _, m := range s.Thread(c.Pub) {
			if m.Dir == DirIn && m.State == StateUnread {
				out = append(out, m)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Stored < out[j].Stored })
	return out
}

// PurgeExpired deletes expired messages and orphaned outbox entries.
func (s *Store) PurgeExpired(now time.Time) int {
	n := 0
	for _, c := range s.ListContacts() {
		for _, m := range s.Thread(c.Pub) {
			if m.Expires > 0 && m.Expires <= now.Unix() {
				s.DeleteMessage(m)
				n++
			}
		}
	}
	for _, e := range s.Outbox() {
		if s.GetMessage(e.Thread, e.ID) == nil {
			s.DeleteOutbox(e.ID)
		}
	}
	return n
}

// ---- outbox ----

func (s *Store) PutOutbox(e *OutboxEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.IsDeleted(e.Thread, e.ID) {
		return nil
	}
	return writeJSON(s.p("outbox", e.ID+".json"), e)
}

func (s *Store) DeleteOutbox(id string) { os.Remove(s.p("outbox", id+".json")) }

func (s *Store) Outbox() []*OutboxEntry {
	files, _ := filepath.Glob(s.p("outbox", "*.json"))
	sort.Strings(files)
	var out []*OutboxEntry
	for _, f := range files {
		var e OutboxEntry
		if readJSON(f, &e) == nil {
			out = append(out, &e)
		}
	}
	return out
}

// ---- peers ----

const maxPeers = 256

func (s *Store) SavePeer(p *Peer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.p("peers", p.Pub+".json")
	if _, err := os.Stat(path); err != nil {
		if files, _ := filepath.Glob(s.p("peers", "*.json")); len(files) >= maxPeers {
			return errors.New("peer table full")
		}
	}
	return writeJSON(path, p)
}

func (s *Store) Peers() []*Peer {
	files, _ := filepath.Glob(s.p("peers", "*.json"))
	var out []*Peer
	for _, f := range files {
		var p Peer
		if readJSON(f, &p) == nil {
			out = append(out, &p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Seen > out[j].Seen })
	return out
}
