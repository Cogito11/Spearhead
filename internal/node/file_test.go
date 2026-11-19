package node

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"spearhead/internal/protocol"
	"spearhead/internal/store"
	"spearhead/internal/transport"
)

func waitFileState(t *testing.T, n *Node, peer, id, state string) *store.Message {
	t.Helper()
	until := time.Now().Add(5 * time.Second)
	for time.Now().Before(until) {
		if m := n.St.GetMessage(peer, id); m != nil && m.State == state {
			return m
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("file did not reach %s: %+v", state, n.St.GetMessage(peer, id))
	return nil
}

func TestEncryptedFileRoundTrip(t *testing.T) {
	for _, size := range []int{0, 3*protocol.FileChunkSize + 17} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			a, b := mk(t, "Alex"), mk(t, "Bob")
			serve(t, b)
			c := befriend(t, a, b)
			payload := bytes.Repeat([]byte{0, 1, 2, 128, 255, 0, 17}, size/7+1)[:size]
			m, err := a.SendFile(c, "../../notes.bin", bytes.NewReader(payload))
			if err != nil {
				t.Fatal(err)
			}
			waitFileState(t, a, key(b), m.Env.ID, store.StateDelivered)
			incoming := b.St.GetMessage(key(a), m.Env.ID)
			if incoming == nil {
				t.Fatal("receiver has no file message")
			}
			info, err := protocol.ParseFile(incoming.Env.Body)
			if err != nil || info.Name != "notes.bin" || info.Size != int64(size) {
				t.Fatalf("metadata %+v %v", info, err)
			}
			got, err := os.ReadFile(b.St.FilePath(key(a), m.Env.ID))
			if err != nil || !bytes.Equal(got, payload) {
				t.Fatalf("content mismatch: %v", err)
			}
			if len(a.St.Outbox()) != 0 {
				t.Fatal("delivered file still queued")
			}
			if ack := rawSend(t, a, addr(b), &incoming.Env, 1)[0]; ack.State != "delivered" {
				t.Fatalf("duplicate file retry %+v", ack)
			}
		})
	}
}

func TestFileRequiresRecipientAcceptance(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	serve(t, b)
	c, err := a.AddContact(b.ID.Pub, "Bob", "Bob", addr(b))
	if err != nil {
		t.Fatal(err)
	}
	m, err := a.SendFile(c, "note.txt", bytes.NewBufferString("private file"))
	if err != nil {
		t.Fatal(err)
	}
	waitFileState(t, a, key(b), m.Env.ID, store.StateFailed)
	if b.St.GetMessage(key(a), m.Env.ID) != nil {
		t.Fatal("unaccepted sender stored an attachment")
	}
	if _, err := os.Stat(b.St.FilePath(key(a), m.Env.ID)); !os.IsNotExist(err) {
		t.Fatal("unaccepted sender wrote a file")
	}
}

func TestQueuedFileRetriesAfterRediscovery(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	serve(t, b)
	c := befriend(t, a, b)
	c.LastAddr = "127.0.0.1:1"
	a.St.SaveContact(c)
	m, err := a.SendFile(c, "later.txt", bytes.NewBufferString("later"))
	if err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		entries := a.St.Outbox()
		if len(entries) > 0 && entries[0].Attempts > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	a.OnPeer(key(b), "Bob", addr(b))
	waitFileState(t, a, key(b), m.Env.ID, store.StateDelivered)
	got, err := os.ReadFile(b.St.FilePath(key(a), m.Env.ID))
	if err != nil || string(got) != "later" {
		t.Fatalf("retry payload %q %v", got, err)
	}
}

func TestFileRejectsCorruptChunks(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	serve(t, b)
	befriend(t, a, b)
	digest := sha256.Sum256([]byte("good"))
	body, _ := json.Marshal(protocol.FileInfo{Name: "note.txt", Size: 4, SHA256: hex.EncodeToString(digest[:])})
	env, err := protocol.New(a.ID, b.ID.Pub, protocol.KindFile, string(body), 0, false, "")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := transport.Dial(addr(b), a.ID, 0, b.ID.Pub, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	raw, _ := json.Marshal(frame{T: "msg", Env: env})
	conn.WriteFrame(raw)
	ack, err := readAck(conn)
	if err != nil || ack.State != "ready" {
		t.Fatalf("prepare %+v %v", ack, err)
	}
	conn.WriteFrame([]byte("evil"))
	ack, err = readAck(conn)
	if err != nil || ack.State != "rejected" {
		t.Fatalf("corrupt file %+v %v", ack, err)
	}
	if b.St.GetMessage(key(a), env.ID) != nil {
		t.Fatal("corrupt message stored")
	}
	if _, err := os.Stat(b.St.FilePath(key(a), env.ID)); !os.IsNotExist(err) {
		t.Fatal("corrupt attachment retained")
	}
}

func TestDeletedAttachmentRetryDoesNotRestoreContent(t *testing.T) {
	a, b := mk(t, "A"), mk(t, "B")
	serve(t, b)
	c := befriend(t, a, b)
	m, err := a.SendFile(c, "remove.bin", bytes.NewReader([]byte("remove this attachment")))
	if err != nil {
		t.Fatal(err)
	}
	waitFileState(t, a, key(b), m.Env.ID, store.StateDelivered)
	if err = b.St.DeleteLocalMessage(key(a), m.Env.ID); err != nil {
		t.Fatal(err)
	}
	ack := rawSend(t, a, addr(b), &m.Env, 1)[0]
	if ack.State != "delivered" {
		t.Fatalf("duplicate acknowledgement %+v", ack)
	}
	if b.St.GetMessage(key(a), m.Env.ID) != nil {
		t.Fatal("attachment message restored")
	}
	if _, err = os.Stat(b.St.FilePath(key(a), m.Env.ID)); !os.IsNotExist(err) {
		t.Fatal("attachment restored")
	}
}
