package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"testing"
	"time"

	"spearhead/internal/identity"
	"spearhead/internal/protocol"
	"spearhead/internal/store"
	"spearhead/internal/transport"
)

func mk(t *testing.T, name string) *Node {
	t.Helper()
	home := t.TempDir()
	if _, err := identity.Create(home, name); err != nil {
		t.Fatal(err)
	}
	n, err := New(home)
	if err != nil {
		t.Fatal(err)
	}
	n.Cfg.ListenPort = 0
	n.Cfg.DefaultRetentionDays = 0
	DialTimeout = time.Second
	return n
}

func serve(t *testing.T, n *Node) context.CancelFunc {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	if err := n.Serve(ctx); err != nil {
		t.Fatal(err)
	}
	return cancel
}

func addr(n *Node) string { return fmt.Sprintf("127.0.0.1:%d", n.Port()) }
func key(n *Node) string  { return identity.Encode(n.ID.Pub) }

// befriend makes a know b (accepted, with address) and b accept a.
func befriend(t *testing.T, a, b *Node) *store.Contact {
	t.Helper()
	c, err := a.AddContact(b.ID.Pub, "bob", "Bob", addr(b))
	if err != nil {
		t.Fatal(err)
	}
	bc, err := b.St.GetContact(key(a))
	if err != nil {
		t.Fatal("request did not arrive:", err)
	}
	bc.Status = store.StatusAccepted
	b.St.SaveContact(bc)
	return c
}

func bodies(ms []*store.Message) (out []string) {
	for _, m := range ms {
		if m.Env.Kind != protocol.KindRequest {
			out = append(out, m.Env.Body)
		}
	}
	return
}

func TestRequestThenDelivery(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	serve(t, b)
	c, err := a.AddContact(b.ID.Pub, "bob", "Bob", addr(b))
	if err != nil {
		t.Fatal(err)
	}
	pc, err := b.St.GetContact(key(a))
	if err != nil || pc.Status != store.StatusPending || pc.TheirName != "Alex" {
		t.Fatalf("unknown sender must land in pending with their name: %+v %v", pc, err)
	}
	if len(b.St.Unread()) != 0 {
		t.Fatal("pending contact's messages must not reach the inbox")
	}
	pc.Status = store.StatusAccepted
	b.St.SaveContact(pc)
	m, err := a.Send(c, "hello", SendOpts{})
	if err != nil || m.State != store.StateDelivered {
		t.Fatalf("expected delivered, got %v %v", m, err)
	}
	if got := bodies(b.St.Unread()); len(got) != 1 || got[0] != "hello" {
		t.Fatalf("bob inbox: %v", got)
	}
	if len(a.St.Outbox()) != 0 {
		t.Fatal("outbox should be empty after ack")
	}
}

func TestQueuedThenDelivered(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	stop := serve(t, b)
	c := befriend(t, a, b)
	stop()
	time.Sleep(50 * time.Millisecond)
	m, _ := a.Send(c, "while you were out", SendOpts{})
	if m.State != store.StateQueued || len(a.St.Outbox()) != 1 {
		t.Fatalf("expected queued with one outbox entry, got %s / %d", m.State, len(a.St.Outbox()))
	}
	b.Cfg.ListenPort = b.Port()
	serve(t, b)
	if err := a.DeliverTo(key(b)); err != nil {
		t.Fatal(err)
	}
	if got := bodies(b.St.Unread()); len(got) != 1 {
		t.Fatalf("bob should now have the message: %v", got)
	}
	if a.St.GetMessage(key(b), m.Env.ID).State != store.StateDelivered {
		t.Fatal("state not updated")
	}
}

func TestKeyMismatchNeverDelivers(t *testing.T) {
	a, b, imposter := mk(t, "Alex"), mk(t, "Bob"), mk(t, "Mallory")
	serve(t, imposter)
	c, _ := a.AddContact(b.ID.Pub, "bob", "Bob", addr(imposter)) // bob's address now answered by mallory
	m, _ := a.Send(c, "secret plans", SendOpts{})
	if m.State != store.StateQueued {
		t.Fatal("message must stay queued")
	}
	if err := a.DeliverTo(key(b)); !errors.Is(err, transport.ErrKeyMismatch) {
		t.Fatalf("want key mismatch, got %v", err)
	}
	if len(imposter.St.Thread(key(a))) != 0 {
		t.Fatal("imposter received data")
	}
}

func TestBlocked(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	serve(t, b)
	c := befriend(t, a, b)
	bc, _ := b.St.GetContact(key(a))
	bc.Status = store.StatusBlocked
	b.St.SaveContact(bc)
	m, _ := a.Send(c, "hi", SendOpts{})
	if m.State != store.StateQueued || len(bodies(b.St.Thread(key(a)))) != 0 {
		t.Fatal("blocked sender must not be delivered to")
	}
}

func TestBurn(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	serve(t, b)
	c := befriend(t, a, b)
	a.Send(c, "normal", SendOpts{})
	m, _ := a.Send(c, "one-time", SendOpts{Burn: true})
	if a.St.GetMessage(key(b), m.Env.ID) != nil {
		t.Fatal("sender must not keep burn message after delivery")
	}
	found := false
	for _, x := range b.St.Unread() {
		if x.Env.Burn && x.Env.Body == "one-time" && x.Env.Prev == "" {
			found = true
		}
	}
	if !found {
		t.Fatal("receiver should hold the burn message until read")
	}
}

func rawSend(t *testing.T, from *Node, to string, env *protocol.Envelope, times int) (acks []frame) {
	t.Helper()
	conn, err := transport.Dial(to, from.ID, from.Port(), nil, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for i := 0; i < times; i++ {
		out, _ := json.Marshal(frame{T: "msg", Env: env})
		conn.WriteFrame(out)
		b, err := conn.ReadFrame()
		if err != nil {
			t.Fatal(err)
		}
		var f frame
		json.Unmarshal(b, &f)
		acks = append(acks, f)
	}
	return
}

func TestDuplicateStoredOnce(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	serve(t, b)
	befriend(t, a, b)
	env, _ := protocol.New(a.ID, b.ID.Pub, protocol.KindChat, "once", 0, false, "")
	acks := rawSend(t, a, addr(b), env, 3)
	for _, f := range acks {
		if f.State != "delivered" {
			t.Fatalf("retry must be re-acknowledged: %+v", f)
		}
	}
	if got := bodies(b.St.Thread(key(a))); len(got) != 1 {
		t.Fatalf("stored %d copies", len(got))
	}
}

func TestSpoofedSenderRejected(t *testing.T) {
	a, b, c := mk(t, "Alex"), mk(t, "Bob"), mk(t, "Carol")
	serve(t, b)
	befriend(t, a, b)
	env, _ := protocol.New(c.ID, b.ID.Pub, protocol.KindChat, "I am Carol", 0, false, "") // valid signature, wrong sender for this connection
	if f := rawSend(t, a, addr(b), env, 1)[0]; f.State != "rejected" {
		t.Fatalf("spoofed sender must be rejected: %+v", f)
	}
	env2, _ := protocol.New(a.ID, b.ID.Pub, protocol.KindChat, "real", 0, false, "")
	env2.Body = "tampered"
	if f := rawSend(t, a, addr(b), env2, 1)[0]; f.State != "rejected" {
		t.Fatalf("tampered envelope must be rejected: %+v", f)
	}
	env3, _ := protocol.New(a.ID, c.ID.Pub, protocol.KindChat, "for carol", 0, false, "")
	if f := rawSend(t, a, addr(b), env3, 1)[0]; f.State != "rejected" {
		t.Fatalf("misaddressed envelope must be rejected: %+v", f)
	}
}

func TestGapDetection(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	serve(t, b)
	c := befriend(t, a, b)
	m1, _ := a.Send(c, "one", SendOpts{})
	bm := b.St.GetMessage(key(a), m1.Env.ID)
	b.St.DeleteMessage(bm) // simulate a lost message
	a.Send(c, "two", SendOpts{})
	got := b.St.Thread(key(a))
	last := got[len(got)-1]
	if last.Env.Body != "two" || !last.Gap {
		t.Fatalf("message after a missing one must be flagged as a gap: %+v", last)
	}
}

func TestPendingLimit(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	serve(t, b)
	c, _ := a.AddContact(b.ID.Pub, "bob", "Bob", addr(b))
	for i := 0; i < maxPendingMsgs+5; i++ {
		a.Send(c, fmt.Sprintf("spam %d", i), SendOpts{})
	}
	if n := len(b.St.Thread(key(a))); n > maxPendingMsgs {
		t.Fatalf("stored %d messages from an unaccepted contact", n)
	}
}

func TestInviteRoundTrip(t *testing.T) {
	a := mk(t, "Alex")
	pub, name, _, err := ParseInvite(a.InviteCode())
	if err != nil || string(pub) != string(a.ID.Pub) || name != "Alex" {
		t.Fatal("invite round trip failed", err)
	}
	if _, _, _, err := ParseInvite("rkt1:!!!"); err == nil {
		t.Fatal("garbage invite accepted")
	}
}

func TestReturnAddressLearnedFromInbound(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	serve(t, a)
	serve(t, b)
	a.AddContact(b.ID.Pub, "bob", "Bob", addr(b))
	pc, err := b.St.GetContact(key(a))
	if err != nil || pc.LastAddr != addr(a) {
		t.Fatalf("bob should learn alex's return address %s, got %+v", addr(a), pc)
	}
}

func TestBlockedContactStopsQueuedDelivery(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	serve(t, b)
	c := befriend(t, a, b)
	c.LastAddr = "127.0.0.1:1"
	if err := a.St.SaveContact(c); err != nil {
		t.Fatal(err)
	}
	m, err := a.Send(c, "queued before blocking", SendOpts{})
	if err != nil || m.State != store.StateQueued {
		t.Fatalf("queue: %v %v", m, err)
	}
	c.Status, c.LastAddr = store.StatusBlocked, addr(b)
	if err := a.St.SaveContact(c); err != nil {
		t.Fatal(err)
	}
	if err := a.DeliverTo(key(b)); err == nil {
		t.Fatal("blocked contact was allowed to deliver")
	}
	if len(bodies(b.St.Thread(key(a)))) != 0 {
		t.Fatal("queued message delivered after blocking")
	}
	stale := *c
	stale.Status = store.StatusAccepted
	if _, err := a.Send(&stale, "stale UI contact", SendOpts{}); err == nil {
		t.Fatal("stale accepted contact bypassed blocking")
	}
}

func TestExpiredOutboxStopsRetryingOfflinePeer(t *testing.T) {
	for _, expired := range []string{"ttl", "max-age"} {
		t.Run(expired, func(t *testing.T) {
			a, b := mk(t, "Alex"), mk(t, "Bob")
			c, err := a.AddContact(b.ID.Pub, "Bob", "Bob", "127.0.0.1:1")
			if err != nil {
				t.Fatal(err)
			}
			a.St.PurgeExpired(time.Now().Add(8 * 24 * time.Hour))
			// Discard the request to isolate the queued chat.
			for _, e := range a.St.Outbox() {
				a.St.DeleteOutbox(e.ID)
			}
			m, err := a.Send(c, "will expire offline", SendOpts{})
			if err != nil {
				t.Fatal(err)
			}
			if expired == "ttl" {
				m.Expires = time.Now().Add(-time.Minute).Unix()
			} else {
				m.Env.TS = time.Now().Add(-maxAge - time.Hour).Unix()
			}
			if err := a.St.SaveMessage(m); err != nil {
				t.Fatal(err)
			}
			if err := a.DeliverTo(key(b)); err != nil {
				t.Fatal("expired entry still tried to connect:", err)
			}
			if len(a.St.Outbox()) != 0 {
				t.Fatal("expired entry still queued")
			}
			if got := a.St.GetMessage(key(b), m.Env.ID); got == nil || got.State != store.StateFailed {
				t.Fatalf("expiry status: %+v", got)
			}
		})
	}
}

func TestPendingFullQueueAcknowledgesDuplicates(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	serve(t, b)
	c, err := a.AddContact(b.ID.Pub, "Bob", "Bob", addr(b))
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i < maxPendingMsgs; i++ {
		if _, err := a.Send(c, fmt.Sprintf("pending %d", i), SendOpts{}); err != nil {
			t.Fatal(err)
		}
	}
	msgs := b.St.Thread(key(a))
	if len(msgs) != maxPendingMsgs {
		t.Fatalf("pending queue size: %d", len(msgs))
	}
	if ack := rawSend(t, a, addr(b), &msgs[len(msgs)-1].Env, 1)[0]; ack.State != "delivered" {
		t.Fatalf("retry rejected at capacity: %+v", ack)
	}
}

func TestChangedDiscoveryAddressRetriesImmediately(t *testing.T) {
	a, b := mk(t, "Alex"), mk(t, "Bob")
	serve(t, b)
	c := befriend(t, a, b)
	c.LastAddr = "127.0.0.1:1"
	if err := a.St.SaveContact(c); err != nil {
		t.Fatal(err)
	}
	m, err := a.Send(c, "found your new address", SendOpts{})
	if err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	a.kick[key(b)] = time.Now()
	a.mu.Unlock()
	a.OnPeer(key(b), "Bob", addr(b))
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		if got := a.St.GetMessage(key(b), m.Env.ID); got != nil && got.State == store.StateDelivered {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("address change was throttled instead of delivering")
}

func TestInviteUsesBoundPort(t *testing.T) {
	a := mk(t, "Alex")
	serve(t, a)
	_, _, addrs, err := ParseInvite(a.InviteCode())
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range addrs {
		_, port, err := net.SplitHostPort(address)
		if err != nil || port != strconv.Itoa(a.Port()) {
			t.Fatalf("invite advertises wrong port: %s (%v)", address, err)
		}
	}
}

func TestDeletedIncomingMessageDoesNotReappearOnRetryOrCreateChainGap(t *testing.T) {
	a, b := mk(t, "A"), mk(t, "B")
	serve(t, a)
	serve(t, b)
	c := befriend(t, a, b)
	first, err := a.Send(c, "Delete locally", SendOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if err = b.St.DeleteLocalMessage(key(a), first.Env.ID); err != nil {
		t.Fatal(err)
	}
	a.St.PutOutbox(&store.OutboxEntry{Thread: key(b), ID: first.Env.ID})
	if err = a.DeliverTo(key(b)); err != nil {
		t.Fatal(err)
	}
	if b.St.GetMessage(key(a), first.Env.ID) != nil {
		t.Fatal("resend restored deleted message")
	}
	next, err := a.Send(c, "Chain remains intact", SendOpts{})
	if err != nil {
		t.Fatal(err)
	}
	received := b.St.GetMessage(key(a), next.Env.ID)
	if received == nil || received.Gap {
		t.Fatal("local deletion broke incoming hash chain")
	}
}
