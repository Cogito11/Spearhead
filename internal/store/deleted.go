package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// A deletion receipt has no message body or attachment. It prevents an incoming
// retry or an in-flight delivery update from bringing a deleted message back,
// and preserves hash-chain continuity without retaining the conversation text.
type deletionReceipt struct {
	ID     string `json:"id"`
	Hash   string `json:"hash,omitempty"`
	Dir    string `json:"dir"`
	Stored int64  `json:"stored"`
}

func validMessageID(id string) bool {
	return len(id) == 32 && strings.Trim(id, "0123456789abcdef") == ""
}
func (s *Store) deletedPath(thread, id string) string {
	return filepath.Join(s.threadDir(thread), ".deleted", id+".json")
}
func (s *Store) IsDeleted(thread, id string) bool {
	if !validMessageID(id) {
		return false
	}
	_, err := os.Stat(s.deletedPath(thread, id))
	return err == nil
}
func (s *Store) HasMessage(thread, id string) bool {
	return s.IsDeleted(thread, id) || s.GetMessage(thread, id) != nil
}
func (s *Store) deletionReceipts(thread string) []deletionReceipt {
	files, _ := filepath.Glob(filepath.Join(s.threadDir(thread), ".deleted", "*.json"))
	receipts := []deletionReceipt{}
	for _, file := range files {
		var receipt deletionReceipt
		if readJSON(file, &receipt) == nil {
			receipts = append(receipts, receipt)
		}
	}
	return receipts
}

// DeleteLocalMessage removes the local content and cancels retries. A network
// transfer already in flight may still have reached the recipient.
func (s *Store) DeleteLocalMessage(thread, id string) error {
	if !validMessageID(id) {
		return errors.New("invalid message id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deleteLocalMessageLocked(thread, id)
}

func (s *Store) deleteLocalMessageLocked(thread, id string) error {
	m := s.GetMessage(thread, id)
	if m == nil && !s.IsDeleted(thread, id) {
		return os.ErrNotExist
	}
	if m != nil {
		receipt := deletionReceipt{ID: id, Dir: m.Dir, Stored: m.Stored}
		if !m.Env.Burn {
			receipt.Hash = m.Hash
		}
		if err := os.MkdirAll(filepath.Dir(s.deletedPath(thread, id)), 0o700); err != nil {
			return err
		}
		if err := writeJSON(s.deletedPath(thread, id), receipt); err != nil {
			return err
		}
	}
	paths := []string{s.p("outbox", id+".json"), s.FilePath(thread, id)}
	records, _ := filepath.Glob(filepath.Join(s.threadDir(thread), "*-"+id+".json"))
	paths = append(paths, records...)
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// OpenReadOnce takes the body and deletes it under the same lock, so concurrent
// callers can reveal it only once. Retries retain only a body-free receipt.
func (s *Store) OpenReadOnce(thread, id string) (string, error) {
	if !validMessageID(id) {
		return "", os.ErrNotExist
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.GetMessage(thread, id)
	if m == nil || m.Dir != DirIn || !m.Env.Burn {
		return "", os.ErrNotExist
	}
	if err := s.deleteLocalMessageLocked(thread, id); err != nil {
		return "", err
	}
	return m.Env.Body, nil
}
