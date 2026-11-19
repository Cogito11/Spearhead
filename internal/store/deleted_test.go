package store

import (
	"os"
	"path/filepath"
	"spearhead/internal/protocol"
	"strings"
	"testing"
)

func TestLocalDeletionCancelsRetriesRemovesContentAndPreservesChain(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m := &Message{Thread: "peer", Dir: DirOut, Hash: strings.Repeat("b", 64), State: StateQueued, Env: protocol.Envelope{ID: strings.Repeat("a", 32), Body: "private conversation text"}}
	if _, err = s.AddMessage(m); err != nil {
		t.Fatal(err)
	}
	s.PutOutbox(&OutboxEntry{Thread: m.Thread, ID: m.Env.ID})
	file := s.FilePath(m.Thread, m.Env.ID)
	os.MkdirAll(filepath.Dir(file), 0700)
	os.WriteFile(file, []byte("private file"), 0600)
	if err = s.DeleteLocalMessage(m.Thread, m.Env.ID); err != nil {
		t.Fatal(err)
	}
	if s.GetMessage(m.Thread, m.Env.ID) != nil || len(s.Outbox()) != 0 {
		t.Fatal("deleted content or retry remains")
	}
	if _, err = os.Stat(file); !os.IsNotExist(err) {
		t.Fatal("attachment remains")
	}
	if s.LastOutHash(m.Thread) != m.Hash || !s.HasMessage(m.Thread, m.Env.ID) {
		t.Fatal("chain or duplicate receipt missing")
	}
	// A stale delivery acknowledgement/update cannot recreate either file.
	s.SaveMessage(m)
	s.PutOutbox(&OutboxEntry{Thread: m.Thread, ID: m.Env.ID})
	if s.GetMessage(m.Thread, m.Env.ID) != nil || len(s.Outbox()) != 0 {
		t.Fatal("stale delivery update resurrected message")
	}
	added, err := s.AddMessage(m)
	if err != nil || added {
		t.Fatal("repeated message resurrected", err)
	}
	receipt, _ := os.ReadFile(s.deletedPath(m.Thread, m.Env.ID))
	if strings.Contains(string(receipt), m.Env.Body) {
		t.Fatal("receipt retained body")
	}
	reopened, _ := Open(s.root)
	if !reopened.IsDeleted(m.Thread, m.Env.ID) {
		t.Fatal("deletion lost on restart")
	}
	if err = s.DeleteLocalMessage(m.Thread, m.Env.ID); err != nil {
		t.Fatal("repeated delete not idempotent", err)
	}
}
func TestDeletedIncomingHashAndInvalidID(t *testing.T) {
	s, _ := Open(t.TempDir())
	m := &Message{Thread: "peer", Dir: DirIn, Hash: strings.Repeat("b", 64), Env: protocol.Envelope{ID: strings.Repeat("c", 32)}}
	s.AddMessage(m)
	if err := s.DeleteLocalMessage(m.Thread, m.Env.ID); err != nil {
		t.Fatal(err)
	}
	if !s.HasIncomingHash(m.Thread, m.Hash) {
		t.Fatal("intentional deletion creates chain gap")
	}
	for _, id := range []string{"", "../../outside", strings.Repeat("G", 32), strings.Repeat("a", 31)} {
		if s.IsDeleted(m.Thread, id) || s.DeleteLocalMessage(m.Thread, id) == nil {
			t.Fatal("invalid id accepted", id)
		}
	}
}
