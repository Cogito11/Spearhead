// Package node ties identity, transport, storage, and discovery together into
// a running SpearHead peer: listener, delivery manager, and contact rules.
package node

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"spearhead/internal/config"
	"spearhead/internal/discovery"
	"spearhead/internal/identity"
	"spearhead/internal/protocol"
	"spearhead/internal/store"
	"spearhead/internal/transport"
)

const (
	idleTimeout    = 60 * time.Second
	maxInbound     = 32
	maxPending     = 50 // unaccepted contacts
	maxPendingMsgs = 20 // stored messages per unaccepted contact
	maxAge         = 7 * 24 * time.Hour
)

var backoff = []time.Duration{5 * time.Second, 15 * time.Second, time.Minute, 5 * time.Minute, 15 * time.Minute}

// DialTimeout bounds outbound connection attempts (variable for tests).
var DialTimeout = 4 * time.Second

type frame struct {
	T      string             `json:"t"`
	Env    *protocol.Envelope `json:"env,omitempty"`
	ID     string             `json:"id,omitempty"`
	State  string             `json:"state,omitempty"`
	Reason string             `json:"reason,omitempty"`
}

type Node struct {
	Home string
	ID   *identity.Identity
	Cfg  *config.Config
	St   *store.Store

	Logf      func(format string, args ...any)
	OnMessage func(*store.Contact, *store.Message)

	ln      net.Listener
	sem     chan struct{}
	cfgMu   sync.RWMutex
	mu      sync.Mutex
	events  atomic.Int64
	queueMu sync.Mutex
	fileSem chan struct{}
	busy    map[string]bool
	kick    map[string]time.Time
}

func New(home string) (*Node, error) {
	id, err := identity.Load(home)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(home)
	if err != nil {
		return nil, err
	}
	st, err := store.Open(home)
	if err != nil {
		return nil, err
	}
	return &Node{Home: home, ID: id, Cfg: cfg, St: st, Logf: func(string, ...any) {},
		sem: make(chan struct{}, maxInbound), fileSem: make(chan struct{}, 2), busy: map[string]bool{}, kick: map[string]time.Time{}}, nil
}

// Settings returns a consistent snapshot for concurrent UI and delivery work.
func (n *Node) Settings() config.Config {
	n.cfgMu.RLock()
	defer n.cfgMu.RUnlock()
	return *n.Cfg
}

func (n *Node) SaveSettings(days int, secretDetection bool) error {
	n.cfgMu.Lock()
	defer n.cfgMu.Unlock()
	cfg := *n.Cfg
	cfg.DefaultRetentionDays, cfg.SecretDetection = days, secretDetection
	if err := config.Save(n.Home, &cfg); err != nil {
		return err
	}
	*n.Cfg = cfg
	return nil
}

// ---------- listening ----------

// Serve binds the TCP listener and accepts connections in the background.
// It returns an error if the port cannot be bound.
func (n *Node) Serve(ctx context.Context) error {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", n.Settings().ListenPort))
	if err != nil {
		return err
	}
	n.ln = ln
	go func() { <-ctx.Done(); ln.Close() }()
	go func() {
		for {
			raw, err := ln.Accept()
			if err != nil {
				return
			}
			select {
			case n.sem <- struct{}{}:
				go func() { defer func() { <-n.sem }(); n.handle(raw) }()
			default:
				raw.Close()
			}
		}
	}()
	return nil
}

// advertPort is the port peers should use to reach us: the bound port if this
// process is listening, otherwise the configured one (a `spearhead listen` daemon
// sharing this config is assumed to be serving it).
func (n *Node) advertPort() int {
	if n.ln != nil {
		return n.Port()
	}
	return n.Settings().ListenPort
}

// Port returns the bound TCP port (0 if not serving).
func (n *Node) Port() int {
	if n.ln == nil {
		return 0
	}
	return n.ln.Addr().(*net.TCPAddr).Port
}

func (n *Node) handle(raw net.Conn) {
	defer raw.Close()
	conn, err := transport.Accept(raw, n.ID, n.advertPort())
	if err != nil {
		n.Logf("inbound handshake from %s failed: %v", raw.RemoteAddr(), err)
		return
	}
	peer := identity.Encode(conn.Peer)
	if c, err := n.St.GetContact(peer); err == nil && c.Status == store.StatusBlocked {
		return // blocked peers are dropped right after the handshake
	}
	for {
		conn.SetDeadline(time.Now().Add(idleTimeout))
		b, err := conn.ReadFrame()
		if err != nil {
			return
		}
		var f frame
		if json.Unmarshal(b, &f) != nil || f.T != "msg" || f.Env == nil {
			return // "bye", or anything unexpected, ends the session
		}
		var state, reason string
		if f.Env.Kind == protocol.KindFile {
			state, reason = n.receiveFile(conn, f.Env)
		} else {
			state, reason = n.receive(conn, f.Env)
		}
		out, _ := json.Marshal(frame{T: "ack", ID: f.Env.ID, State: state, Reason: reason})
		if conn.WriteFrame(out) != nil {
			return
		}
	}
}

func (n *Node) expiry(ts, ttl int64) int64 {
	if ttl > 0 {
		return ts + ttl
	}
	if d := n.Settings().DefaultRetentionDays; d > 0 {
		return time.Now().Add(time.Duration(d) * 24 * time.Hour).Unix()
	}
	return 0
}

func (n *Node) pendingCount() int {
	k := 0
	for _, c := range n.St.ListContacts() {
		if c.Status == store.StatusPending {
			k++
		}
	}
	return k
}

// receive validates and stores one incoming envelope (design doc, Section 22.5).
func (n *Node) receive(conn *transport.Conn, env *protocol.Envelope) (state, reason string) {
	peer := identity.Encode(conn.Peer)
	if err := env.Verify(); err != nil {
		return "rejected", "invalid envelope: " + err.Error()
	}
	if env.From != peer {
		return "rejected", "sender does not match authenticated peer"
	}
	if !env.AddressedTo(n.ID.Pub) {
		return "rejected", "not addressed to this node"
	}
	now := time.Now()
	if env.TTL > 0 && (env.TS <= now.Unix()-env.TTL) {
		return "rejected", "message expired"
	}
	ct, err := n.St.GetContact(peer)
	if err != nil { // unknown sender: pending queue only
		if n.pendingCount() >= maxPending {
			return "rejected", "recipient has too many pending requests"
		}
		label := conn.PeerName
		if label == "" {
			label = identity.Fingerprint(conn.Peer)
		}
		ct = &store.Contact{Pub: peer, Label: label, Status: store.StatusPending, FirstSeen: now.Unix()}
	}
	if ct.Status == store.StatusBlocked {
		return "rejected", "blocked"
	}
	if env.Kind == protocol.KindFile && ct.Status != store.StatusAccepted {
		return "rejected", "files require an accepted contact"
	}
	// Re-acknowledge retries before enforcing the pending queue limit.
	if n.St.HasMessage(peer, env.ID) {
		return "delivered", ""
	}
	if ct.Status == store.StatusPending && len(n.St.Thread(peer)) >= maxPendingMsgs {
		return "rejected", "pending message limit reached"
	}
	known := err == nil
	ct.LastSeen = now.Unix()
	if conn.PeerPort > 0 { // return address: their real IP plus the port they listen on
		if host, _, err := net.SplitHostPort(conn.RemoteAddr().String()); err == nil {
			ct.LastAddr = net.JoinHostPort(host, strconv.Itoa(conn.PeerPort))
		}
	}
	ct.Device, ct.Platform = conn.PeerDevice, conn.PeerPlatform
	if conn.PeerName != "" {
		ct.TheirName = conn.PeerName
	}
	if known {
		err = n.St.UpdateContact(peer, func(c *store.Contact) {
			c.LastSeen, c.TheirName = ct.LastSeen, ct.TheirName
			c.Device, c.Platform = ct.Device, ct.Platform
			if ct.LastAddr != "" {
				c.LastAddr = ct.LastAddr
			}
		})
		ct, _ = n.St.GetContact(peer)
	} else {
		err = n.St.SaveContact(ct)
	}
	if err != nil || ct == nil {
		return "rejected", "storage error"
	}
	if ct.Status == store.StatusBlocked {
		return "rejected", "blocked"
	}
	if env.Kind == protocol.KindRequest && ct.Status == store.StatusAccepted {
		return "delivered", "" // already a contact; nothing to queue
	}
	m := &store.Message{Env: *env, Dir: store.DirIn, State: store.StateUnread, Hash: env.Hash(),
		Thread: peer, Expires: n.expiry(env.TS, env.TTL)}
	if env.Prev != "" && !n.St.HasIncomingHash(peer, env.Prev) {
		m.Gap = true
	}
	added, err := n.St.AddMessage(m)
	if err != nil {
		return "rejected", "storage error"
	}
	if added {
		n.events.Add(1)
	}
	if added && n.OnMessage != nil {
		n.OnMessage(ct, m)
	}
	return "delivered", "" // duplicates are acknowledged again, stored once
}

// ---------- sending ----------

type SendOpts struct {
	Kind string
	TTL  int64
	Burn bool
}

// Send signs and queues a message, then makes an immediate delivery attempt.
// Failure to deliver now is not an error: the message stays in the outbox.
func (n *Node) Send(c *store.Contact, body string, o SendOpts) (*store.Message, error) {
	if c.Status != store.StatusAccepted {
		return nil, errors.New("contact is not accepted")
	}
	fresh, err := n.St.GetContact(c.Pub)
	if err != nil || fresh.Status != store.StatusAccepted {
		return nil, errors.New("contact is not accepted")
	}
	c = fresh
	pub, err := identity.Decode(c.Pub)
	if err != nil {
		return nil, err
	}
	if o.Kind == "" {
		o.Kind = protocol.KindChat
	}
	n.queueMu.Lock()
	locked := true
	defer func() {
		if locked {
			n.queueMu.Unlock()
		}
	}()
	prev := ""
	if !o.Burn {
		prev = n.St.LastOutHash(c.Pub)
	}
	env, err := protocol.New(n.ID, pub, o.Kind, body, o.TTL, o.Burn, prev)
	if err != nil {
		return nil, err
	}
	m := &store.Message{Env: *env, Dir: store.DirOut, State: store.StateQueued, Hash: env.Hash(),
		Thread: c.Pub, Expires: n.expiry(env.TS, env.TTL)}
	if _, err := n.St.AddMessage(m); err != nil {
		return nil, err
	}
	if err := n.St.PutOutbox(&store.OutboxEntry{Thread: c.Pub, ID: env.ID}); err != nil {
		return nil, err
	}
	n.queueMu.Unlock()
	locked = false
	if err := n.DeliverTo(c.Pub); err != nil {
		n.Logf("queued (not delivered yet): %v", err)
	}
	if fresh := n.St.GetMessage(c.Pub, env.ID); fresh != nil {
		m = fresh
	} else if o.Burn {
		m.State = store.StateDelivered // burn messages are deleted from our side once delivered
	}
	return m, nil
}

func (n *Node) bump(e *store.OutboxEntry) {
	e.Attempts++
	i := e.Attempts - 1
	if i >= len(backoff) {
		i = len(backoff) - 1
	}
	e.NextTry = time.Now().Add(backoff[i]).Unix()
	n.St.PutOutbox(e)
}

// DeliverTo sends every queued message for one peer over a single connection.
func (n *Node) DeliverTo(peer string) error {
	n.mu.Lock()
	if n.busy[peer] {
		n.mu.Unlock()
		return nil
	}
	n.busy[peer] = true
	n.mu.Unlock()
	defer func() { n.mu.Lock(); delete(n.busy, peer); n.mu.Unlock() }()

	ct, err := n.St.GetContact(peer)
	if err != nil {
		return err
	}
	if ct.Status != store.StatusAccepted {
		return errors.New("contact is not accepted")
	}
	type item struct {
		e *store.OutboxEntry
		m *store.Message
	}
	var items []item
	for _, e := range n.St.Outbox() {
		if e.Thread != peer {
			continue
		}
		m := n.St.GetMessage(peer, e.ID)
		if m == nil {
			n.St.DeleteOutbox(e.ID)
			continue
		}
		now := time.Now()
		if (m.Expires > 0 && m.Expires <= now.Unix()) || now.Sub(time.Unix(m.Env.TS, 0)) > maxAge {
			m.State, m.Reason = store.StateFailed, "message expired before delivery"
			if err := n.St.SaveMessage(m); err != nil {
				return err
			}
			n.St.DeleteOutbox(e.ID)
			continue
		}
		items = append(items, item{e, m})
	}
	if len(items) == 0 {
		return nil
	}
	sort.Slice(items, func(i, j int) bool { return items[i].m.Stored < items[j].m.Stored })
	fail := func(err error) error {
		for _, it := range items {
			n.bump(it.e)
		}
		return err
	}
	if ct.LastAddr == "" {
		return fail(errors.New("no known address for " + ct.Label))
	}
	pub, err := identity.Decode(peer)
	if err != nil {
		return err
	}
	conn, err := transport.Dial(ct.LastAddr, n.ID, n.advertPort(), pub, DialTimeout)
	if err != nil {
		return fail(fmt.Errorf("%s: %w", ct.Label, err))
	}
	defer conn.Close()
	n.St.UpdateContact(peer, func(c *store.Contact) {
		c.LastSeen = time.Now().Unix()
		c.Device, c.Platform = conn.PeerDevice, conn.PeerPlatform
		if conn.PeerName != "" {
			c.TheirName = conn.PeerName
		}
	})

	for i, it := range items {
		if n.St.IsDeleted(peer, it.m.Env.ID) {
			n.St.DeleteOutbox(it.e.ID)
			continue
		}
		if time.Since(time.Unix(it.m.Env.TS, 0)) > maxAge {
			it.m.State, it.m.Reason = store.StateFailed, "undeliverable for 7 days"
			n.St.SaveMessage(it.m)
			n.St.DeleteOutbox(it.e.ID)
			continue
		}
		out, _ := json.Marshal(frame{T: "msg", Env: &it.m.Env})
		conn.SetDeadline(time.Now().Add(15 * time.Second))
		if err := conn.WriteFrame(out); err != nil {
			for _, rest := range items[i:] {
				n.bump(rest.e)
			}
			return err
		}
		var ack frame
		if it.m.Env.Kind == protocol.KindFile {
			ack, err = n.deliverFile(conn, it.m)
		} else {
			ack, err = readAck(conn)
		}

		if err != nil || ack.T != "ack" || ack.ID != it.m.Env.ID || (ack.State != "delivered" && ack.State != "rejected") {
			for _, rest := range items[i:] {
				n.bump(rest.e)
			}
			return errors.New("no valid acknowledgement from " + ct.Label)
		}
		n.St.DeleteOutbox(it.e.ID)
		if ack.State == "delivered" {
			if it.m.Env.Burn {
				n.St.DeleteMessage(it.m) // burn: sender keeps nothing
				continue
			}
			it.m.State = store.StateDelivered
		} else {
			it.m.State, it.m.Reason = store.StateFailed, ack.Reason
		}
		n.St.SaveMessage(it.m)
	}
	bye, _ := json.Marshal(frame{T: "bye"})
	conn.WriteFrame(bye)
	return nil
}

// FlushOutbox retries every peer with due entries, in parallel.
func (n *Node) FlushOutbox() {
	due := map[string]bool{}
	for _, e := range n.St.Outbox() {
		if e.NextTry <= time.Now().Unix() {
			due[e.Thread] = true
		}
	}
	var wg sync.WaitGroup
	for peer := range due {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			if err := n.DeliverTo(p); err != nil {
				n.Logf("delivery retry: %v", err)
			}
		}(peer)
	}
	wg.Wait()
}

// ---------- discovery and daemon ----------

// OnPeer records a beacon and, if it is a known contact, refreshes its address
// and retries any queued messages (at most every 30 s per peer).
func (n *Node) OnPeer(pub, name, addr string, info ...string) {
	now := time.Now()
	p := &store.Peer{Pub: pub, Name: name, Addr: addr, Seen: now.Unix()}
	if len(info) == 2 {
		p.Device, p.Platform = info[0], info[1]
	}
	n.St.SavePeer(p)
	ct, err := n.St.GetContact(pub)
	if err != nil || ct.Status == store.StatusBlocked {
		return
	}
	changed := ct.LastAddr != addr
	if changed || name != ct.TheirName || p.Device != ct.Device || now.Unix()-ct.LastSeen > 15 {
		n.St.UpdateContact(pub, func(c *store.Contact) {
			if c.Status != store.StatusBlocked {
				c.LastAddr, c.LastSeen, c.TheirName = addr, now.Unix(), name
				if len(info) == 2 {
					c.Device, c.Platform = p.Device, p.Platform
				}
			}
		})
	}
	n.mu.Lock()
	recent := !changed && now.Sub(n.kick[pub]) < 30*time.Second
	if !recent {
		n.kick[pub] = now
	}
	n.mu.Unlock()
	if recent {
		return
	}
	for _, e := range n.St.Outbox() {
		if e.Thread == pub {
			go func() {
				if err := n.DeliverTo(pub); err != nil {
					n.Logf("delivery to %s: %v", ct.Label, err)
				}
			}()
			return
		}
	}
}

// Run serves, announces, and retries until ctx is cancelled.
func (n *Node) Run(ctx context.Context) error {
	if err := n.Serve(ctx); err != nil {
		return err
	}
	n.Run2(ctx)
	return nil
}

// Run2 announces and retries on an already-serving node until ctx is cancelled.
func (n *Node) Run2(ctx context.Context) {
	if n.Settings().Discovery {
		go discovery.RunWithInfo(ctx, n.Settings().DiscoveryPort, n.Port(), n.ID, func(b discovery.Beacon, addr string) { n.OnPeer(b.Pub, b.Name, addr, b.Device, b.Platform) },
			func(err error) { n.Logf("discovery listener: %v", err) })
	}
	n.St.PurgeExpired(time.Now())
	n.FlushOutbox()
	maintenance := time.NewTicker(5 * time.Minute)
	defer maintenance.Stop()
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			n.FlushOutbox()
		case <-maintenance.C:
			n.St.PurgeExpired(time.Now())
		}
	}
}

// ---------- contacts ----------

// Probe connects to addr only to learn which identity answers there.
func (n *Node) Probe(addr string) (ed25519.PublicKey, string, error) {
	conn, err := transport.Dial(addr, n.ID, n.advertPort(), nil, DialTimeout)
	if err != nil {
		return nil, "", err
	}
	defer conn.Close()
	bye, _ := json.Marshal(frame{T: "bye"})
	conn.WriteFrame(bye)
	return conn.Peer, conn.PeerName, nil
}

// AddContact creates (or promotes) an accepted contact and, for new contacts,
// sends a contact request so it appears in the other side's requests queue.
func (n *Node) AddContact(pub ed25519.PublicKey, label, theirName, addr string) (*store.Contact, error) {
	key := identity.Encode(pub)
	if key == identity.Encode(n.ID.Pub) {
		return nil, errors.New("that is your own identity")
	}
	c, err := n.St.GetContact(key)
	isNew := err != nil
	if isNew {
		c = &store.Contact{Pub: key, FirstSeen: time.Now().Unix()}
	}
	if c.Status == store.StatusBlocked {
		return nil, errors.New("contact is blocked; unblock first")
	}
	c.Status = store.StatusAccepted
	if label != "" {
		c.Label = label
	} else if c.Label == "" {
		c.Label = theirName
		if c.Label == "" {
			c.Label = identity.Fingerprint(pub)
		}
	}
	if theirName != "" {
		c.TheirName = theirName
	}
	if addr != "" {
		c.LastAddr = addr
	}
	if err := n.St.SaveContact(c); err != nil {
		return nil, err
	}
	if isNew {
		if _, err := n.Send(c, "Contact request from "+n.ID.DisplayName(), SendOpts{Kind: protocol.KindRequest}); err != nil {
			n.Logf("could not queue contact request: %v", err)
		}
	}
	return c, nil
}

// ActivityVersion is a cheap counter for the hidden desktop client.
func (n *Node) ActivityVersion() int64 { return n.events.Load() }
