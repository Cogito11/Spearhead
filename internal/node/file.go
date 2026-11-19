package node

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"spearhead/internal/identity"
	"spearhead/internal/protocol"
	"spearhead/internal/store"
	"spearhead/internal/transport"
)

// SendFile stages an attachment on disk and queues it without retaining the
// contents in RAM. Background delivery uses the same authenticated channel.
func (n *Node) SendFile(c *store.Contact, name string, r io.Reader) (*store.Message, error) {
	fresh, err := n.St.GetContact(c.Pub)
	if err != nil || fresh.Status != store.StatusAccepted {
		return nil, errors.New("contact is not accepted")
	}
	staged, size, hash, err := n.St.StageFile(r)
	if err != nil {
		return nil, err
	}
	defer os.Remove(staged)
	pub, err := identity.Decode(c.Pub)
	if err != nil {
		return nil, err
	}
	info := protocol.FileInfo{Name: protocol.CleanFileName(name), Size: size, SHA256: hash}
	body, _ := json.Marshal(info)
	n.queueMu.Lock()
	env, err := protocol.New(n.ID, pub, protocol.KindFile, string(body), 0, false, n.St.LastOutHash(c.Pub))
	if err != nil {
		n.queueMu.Unlock()
		return nil, err
	}
	if err = n.St.CommitFile(staged, c.Pub, env.ID); err != nil {
		n.queueMu.Unlock()
		return nil, err
	}
	m := &store.Message{Env: *env, Dir: store.DirOut, State: store.StateQueued, Hash: env.Hash(), Thread: c.Pub, Expires: n.expiry(env.TS, 0)}
	if _, err = n.St.AddMessage(m); err == nil {
		err = n.St.PutOutbox(&store.OutboxEntry{Thread: c.Pub, ID: env.ID})
	}
	n.queueMu.Unlock()
	if err != nil {
		os.Remove(n.St.FilePath(c.Pub, env.ID))
		return nil, err
	}
	go func() {
		if err := n.DeliverTo(c.Pub); err != nil {
			n.Logf("file queued: %v", err)
		}
	}()
	return m, nil
}

func readAck(conn *transport.Conn) (frame, error) {
	raw, err := conn.ReadFrame()
	if err != nil {
		return frame{}, err
	}
	var ack frame
	if json.Unmarshal(raw, &ack) != nil || ack.T != "ack" {
		return frame{}, errors.New("invalid acknowledgement")
	}
	return ack, nil
}

// deliverFile is called after the signed metadata envelope has been sent.
func (n *Node) deliverFile(conn *transport.Conn, m *store.Message) (frame, error) {
	ack, err := readAck(conn)
	if err != nil {
		return ack, err
	}
	if ack.ID != m.Env.ID {
		return ack, errors.New("file acknowledgement mismatch")
	}
	if ack.State != "ready" {
		return ack, nil
	}
	info, err := protocol.ParseFile(m.Env.Body)
	if err != nil {
		return frame{}, err
	}
	f, err := os.Open(n.St.FilePath(m.Thread, m.Env.ID))
	if err != nil {
		return frame{}, err
	}
	defer f.Close()
	remaining := info.Size
	buf := make([]byte, protocol.FileChunkSize)
	for remaining > 0 {
		chunk := int64(len(buf))
		if chunk > remaining {
			chunk = remaining
		}
		if _, err := io.ReadFull(f, buf[:chunk]); err != nil {
			return frame{}, err
		}
		conn.SetDeadline(time.Now().Add(30 * time.Second))
		if err := conn.WriteFrame(buf[:chunk]); err != nil {
			return frame{}, err
		}
		remaining -= chunk
	}
	conn.SetDeadline(time.Now().Add(30 * time.Second))
	return readAck(conn)
}

type fileReader struct {
	conn      *transport.Conn
	remaining int64
	chunk     []byte
}

func (r *fileReader) Read(dst []byte) (int, error) {
	if len(r.chunk) == 0 {
		if r.remaining == 0 {
			return 0, io.EOF
		}
		r.conn.SetDeadline(time.Now().Add(30 * time.Second))
		b, err := r.conn.ReadFrame()
		if err != nil {
			return 0, err
		}
		if len(b) == 0 || len(b) > protocol.FileChunkSize || int64(len(b)) > r.remaining {
			return 0, errors.New("invalid file chunk")
		}
		r.chunk = b
		r.remaining -= int64(len(b))
	}
	n := copy(dst, r.chunk)
	r.chunk = r.chunk[n:]
	return n, nil
}

func (n *Node) receiveFile(conn *transport.Conn, env *protocol.Envelope) (string, string) {
	peer := identity.Encode(conn.Peer)
	if err := env.Verify(); err != nil {
		return "rejected", "invalid envelope"
	}
	if env.From != peer || !env.AddressedTo(n.ID.Pub) {
		return "rejected", "wrong sender or recipient"
	}
	c, err := n.St.GetContact(peer)
	if err != nil || c.Status != store.StatusAccepted {
		return "rejected", "files require an accepted contact"
	}
	if n.St.IsDeleted(peer, env.ID) {
		return "delivered", ""
	}
	if n.St.GetMessage(peer, env.ID) != nil {
		if _, err := os.Stat(n.St.FilePath(peer, env.ID)); err == nil {
			return "delivered", ""
		}
		return "rejected", "attachment no longer available"
	}
	info, err := protocol.ParseFile(env.Body)
	if err != nil {
		return "rejected", err.Error()
	}
	// Limit concurrent disk streams independently of inexpensive chat sessions.
	select {
	case n.fileSem <- struct{}{}:
		defer func() { <-n.fileSem }()
	default:
		return "retry", "recipient is busy receiving files"
	}
	ready, _ := json.Marshal(frame{T: "ack", ID: env.ID, State: "ready"})
	if err := conn.WriteFrame(ready); err != nil {
		return "rejected", "connection closed"
	}
	staged, size, hash, err := n.St.StageFile(&fileReader{conn: conn, remaining: info.Size})
	if err != nil {
		return "rejected", "incomplete file transfer"
	}
	defer os.Remove(staged)
	if size != info.Size || hash != info.SHA256 {
		return "rejected", "file integrity check failed"
	}
	c, err = n.St.GetContact(peer)
	if err != nil || c.Status != store.StatusAccepted {
		return "rejected", "contact is no longer accepted"
	}
	if err = n.St.CommitFile(staged, peer, env.ID); err != nil {
		return "rejected", "storage error"
	}
	state, reason := n.receive(conn, env)
	if state != "delivered" || n.St.IsDeleted(peer, env.ID) {
		os.Remove(n.St.FilePath(peer, env.ID))
	}
	return state, reason
}

func filePreview(body string) string {
	f, err := protocol.ParseFile(body)
	if err != nil {
		return "Attachment"
	}
	return fmt.Sprintf("File: %s", f.Name)
}
