// Command spearhead is a serverless, peer-to-peer, end-to-end encrypted messenger
// for small teams and groups. See the design document for the full specification.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"spearhead/internal/config"
	"spearhead/internal/identity"
	"spearhead/internal/node"
	"spearhead/internal/protocol"
	"spearhead/internal/safety"
	"spearhead/internal/store"
)

const usage = `spearhead - private peer-to-peer messenger (no accounts, no servers)

Just run ` + "`spearhead`" + ` with no arguments: it sets you up and opens the native Qt Quick desktop.

Usage: spearhead [--home DIR] <command> [flags] [args]

Setup:      init [--name N] [--port P]   whoami   invite   nearby
Contacts:   add <invite|host:port|name> [label]   contacts   requests
            accept|decline|block|unblock <who>    verify <who>
Messaging:  send [--burn] [--ttl SECS] [--force] <who> <message>   (or pipe text on stdin)
            inbox   chat <who>   log <who> [--n N] [--grep T] [--since 2d]   search <term>
Other:      listen (headless)   status   doctor (check your setup)   help [command]

<who> is a contact label, fingerprint (or its prefix), or key prefix.
Put flags before the message, or use -- to end flags.`

var stdin = bufio.NewReader(os.Stdin)

func die(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "spearhead: "+f+"\n", a...)
	pause()
	os.Exit(1)
}

func check(err error) {
	if err != nil {
		die("%v", err)
	}
}

func interactive() bool {
	st, err := os.Stdin.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func confirm(prompt string) bool {
	if !interactive() {
		return false
	}
	fmt.Print(prompt + " [y/N] ")
	line, _ := stdin.ReadString('\n')
	l := strings.ToLower(strings.TrimSpace(line))
	return l == "y" || l == "yes"
}

// parseArgs allows flags before, between, or after positionals. Everything
// after a literal "--" is returned separately, untouched.
func parseArgs(fs *flag.FlagSet, args []string) (pos, rest []string) {
	for i, a := range args {
		if a == "--" {
			rest, args = args[i+1:], args[:i]
			break
		}
	}
	for {
		if err := fs.Parse(args); err != nil {
			os.Exit(2)
		}
		args = fs.Args()
		if len(args) == 0 {
			return
		}
		pos, args = append(pos, args[0]), args[1:]
	}
}

func main() {
	args := os.Args[1:]
	homeFlag := ""
	for len(args) >= 2 && args[0] == "--home" {
		homeFlag, args = args[1], args[2:]
	}
	home := config.Home(homeFlag)
	if len(args) == 0 || args[0] == "app" || args[0] == "--background" {
		if len(args) == 0 {
			cmdApp(home)
		} else if args[0] == "app" {
			cmdApp(home, args[1:]...)
		} else {
			cmdApp(home, args...)
		}
		return
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		cmdHelp(args[1:])
		return
	}
	cmd, rest := args[0], args[1:]
	if cmd == "doctor" {
		cmdDoctor(home)
		return
	}
	known := false
	for _, c := range commands {
		known = known || c == cmd
	}
	if !known {
		if s := suggest(cmd); s != "" {
			die("unknown command %q. Did you mean `spearhead %s`? (`spearhead help` lists everything)", cmd, s)
		}
		die("unknown command %q (try `spearhead help`)", cmd)
	}
	if cmd == "init" {
		cmdInit(home, rest)
		return
	}
	n, err := node.New(home)
	check(err)
	n.Logf = func(f string, a ...any) { fmt.Fprintf(os.Stderr, time.Now().Format("15:04:05 ")+f+"\n", a...) }

	switch cmd {
	case "whoami":
		cmdWhoami(n)
	case "invite":
		fmt.Println(n.InviteCode())
	case "nearby":
		cmdNearby(n)
	case "add":
		cmdAdd(n, rest)
	case "contacts":
		cmdContacts(n)
	case "requests":
		cmdRequests(n)
	case "accept", "decline", "block", "unblock":
		cmdResolve(n, cmd, rest)
	case "verify":
		cmdVerify(n, rest)
	case "send":
		cmdSend(n, rest)
	case "inbox":
		cmdInbox(n)
	case "chat":
		cmdChat(n, rest)
	case "log":
		cmdLog(n, rest)
	case "search":
		cmdSearch(n, rest)
	case "listen":
		cmdListen(n)
	case "status":
		cmdStatus(n)
	default:
		if s := suggest(cmd); s != "" {
			die("unknown command %q. Did you mean `spearhead %s`? (`spearhead help` lists everything)", cmd, s)
		}
		die("unknown command %q (try `spearhead help`)", cmd)
	}
}

// ---------- helpers ----------

func ago(ts int64) string {
	if ts == 0 {
		return "never"
	}
	d := time.Since(time.Unix(ts, 0))
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(d.Hours()/24))
}

// clean removes terminal control characters from untrusted text so a peer
// cannot inject escape sequences into the reader's terminal.
func clean(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 32 || (r >= 127 && r < 160) {
			return '?'
		}
		return r
	}, s)
}

func fp(c *store.Contact) string {
	pub, err := identity.Decode(c.Pub)
	if err != nil {
		return "?"
	}
	return identity.Fingerprint(pub)
}

func mark(c *store.Contact) string {
	switch {
	case c.Status == store.StatusBlocked:
		return "blocked"
	case c.Status == store.StatusPending:
		return "pending"
	case c.Verified:
		return "verified"
	}
	return "unverified"
}

func render(n *node.Node, c *store.Contact, m *store.Message) {
	who := c.Label
	if m.Dir == store.DirOut {
		who = "me -> " + c.Label
	} else {
		who += " (" + mark(c) + ")"
	}
	tag := ""
	if m.Env.Burn {
		tag += " [burn: shown once]"
	}
	if m.Gap {
		tag += " [gap: earlier message missing]"
	}
	state := ""
	if m.Dir == store.DirOut && m.State != store.StateDelivered {
		state = " {" + m.State
		if m.Reason != "" {
			state += ": " + m.Reason
		}
		state += "}"
	}
	fmt.Printf("[%s] %s%s%s\n", time.Unix(m.Env.TS, 0).Local().Format("2006-01-02 15:04"), clean(who), tag, state)
	for _, ln := range strings.Split(clean(m.Env.Body), "\n") {
		fmt.Println("  " + ln)
	}
}

func find(n *node.Node, q string) *store.Contact {
	c, err := n.St.FindContact(q)
	if err != nil {
		die("%v (see `spearhead contacts` for names, or `spearhead nearby` to find people)", err)
	}
	return c
}

func accepted(n *node.Node, q string) *store.Contact {
	c := find(n, q)
	if c.Status != store.StatusAccepted {
		if c.Status == store.StatusPending {
			die("%s is waiting for your OK. Run `spearhead accept %s` first.", c.Label, c.Label)
		}
		die("%s is %s, so you can't message them. (`spearhead unblock %s` to undo.)", c.Label, c.Status, c.Label)
	}
	return c
}

// guardSecrets scans text and asks before letting likely credentials through.
func guardSecrets(n *node.Node, text string, force bool) {
	if !n.Settings().SecretDetection || force {
		return
	}
	fs := safety.Scan(text)
	if len(fs) == 0 {
		return
	}
	fmt.Fprintln(os.Stderr, "This looks like it contains a secret:")
	for _, f := range fs {
		fmt.Fprintf(os.Stderr, "  - %s: %s\n", f.Rule, f.Sample)
	}
	fmt.Fprintln(os.Stderr, "Consider `send --burn` so it is deleted after they read it.")
	if !confirm("Send anyway?") {
		die("not sent (use --force to override)")
	}
}

func reportSend(m *store.Message, c *store.Contact) {
	switch m.State {
	case store.StateDelivered:
		fmt.Printf("delivered to %s\n", c.Label)
	case store.StateFailed:
		fmt.Printf("rejected by %s: %s\n", c.Label, m.Reason)
	default:
		fmt.Printf("queued for %s; it will be delivered when their node is reachable (`spearhead listen` retries)\n", c.Label)
	}
}

// ---------- commands ----------

func cmdInit(home string, rest []string) {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	name := fs.String("name", "", "display name")
	port := fs.Int("port", 0, "listen port (default 7777)")
	parseArgs(fs, rest)
	if *name == "" {
		*name = os.Getenv("USERNAME")
		if *name == "" {
			*name = os.Getenv("USER")
		}
		if *name == "" {
			*name = "spearhead-user"
		}
	}
	id, err := identity.Create(home, *name)
	check(err)
	cfg, err := config.Load(home)
	check(err)
	if *port != 0 {
		cfg.ListenPort = *port
	}
	check(config.Save(home, cfg))
	fmt.Printf("Created identity for %s\nFingerprint: %s\nData directory: %s\n\n", id.Name, identity.Fingerprint(id.Pub), home)
	fmt.Println("Next: run `spearhead listen` (keep it running), then `spearhead nearby` or `spearhead invite`.")
	fmt.Printf("Allow inbound TCP %d (private network) and UDP %d in the firewall.\n", cfg.ListenPort, cfg.DiscoveryPort)
}

func cmdWhoami(n *node.Node) {
	fmt.Printf("Name:        %s\nFingerprint: %s\nPublic key:  %s\nListen port: %d\nData dir:    %s\n",
		n.ID.DisplayName(), identity.Fingerprint(n.ID.Pub), identity.Encode(n.ID.Pub), n.Settings().ListenPort, n.Home)
}

func cmdNearby(n *node.Node) {
	fmt.Printf("%-27s %-16s %-22s %s\n", "FINGERPRINT", "NAME", "ADDRESS", "SEEN")
	k := 0
	for _, p := range n.St.Peers() {
		if time.Since(time.Unix(p.Seen, 0)) > 90*time.Second {
			continue
		}
		pub, err := identity.Decode(p.Pub)
		if err != nil {
			continue
		}
		extra := ""
		if c, err := n.St.GetContact(p.Pub); err == nil {
			extra = "  [" + c.Label + ": " + c.Status + "]"
		}
		fmt.Printf("%-27s %-16s %-22s %s%s\n", identity.Fingerprint(pub), clean(p.Name), p.Addr, ago(p.Seen), extra)
		k++
	}
	if k == 0 {
		fmt.Println("(none seen in the last 90s; is `spearhead listen` running here and on the other machine? Beacons do not cross VLANs, so try `spearhead add host:port`.)")
	}
}

func cmdAdd(n *node.Node, rest []string) {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	yes := fs.Bool("yes", false, "skip the trust confirmation")
	pos, _ := parseArgs(fs, rest)
	if len(pos) < 1 {
		die("usage: spearhead add <invite-code | host:port | nearby name/fingerprint> [label]")
	}
	target, label := pos[0], ""
	if len(pos) > 1 {
		label = pos[1]
	}
	var pub ed25519.PublicKey
	var name, addr string
	switch {
	case node.IsInvite(target):
		var addrs []string
		var err error
		pub, name, addrs, err = node.ParseInvite(target)
		check(err)
		for _, a := range addrs { // keep the first address that really answers as this key
			if p, _, err := n.Probe(a); err == nil && bytes.Equal(p, pub) {
				addr = a
				break
			}
		}
		if addr == "" && len(addrs) > 0 {
			addr = addrs[0]
			fmt.Println("(no address in the invite answered right now; keeping the first one)")
		}
	default:
		if _, _, err := net.SplitHostPort(target); err == nil {
			var err error
			pub, name, err = n.Probe(target)
			check(err)
			addr = target
		} else {
			var match *store.Peer
			for _, p := range n.St.Peers() {
				if time.Since(time.Unix(p.Seen, 0)) > 90*time.Second {
					continue
				}
				k, _ := identity.Decode(p.Pub)
				if strings.EqualFold(p.Name, target) || strings.HasPrefix(identity.Fingerprint(k), strings.ToLower(target)) {
					if match != nil {
						die("%q matches more than one nearby node; use the fingerprint", target)
					}
					match = p
				}
			}
			if match == nil {
				die("nothing nearby matches %q (see `spearhead nearby`)", target)
			}
			var err error
			pub, err = identity.Decode(match.Pub)
			check(err)
			name, addr = match.Name, match.Addr
		}
		fmt.Printf("Found %q with fingerprint %s\n", clean(name), identity.Fingerprint(pub))
		if !*yes && !confirm("Trust this identity? (Verify it afterwards with `spearhead verify`.)") {
			die("aborted (use --yes to skip the prompt)")
		}
	}
	c, err := n.AddContact(pub, label, name, addr)
	check(err)
	fmt.Printf("Added %s (%s, %s). ", c.Label, fp(c), mark(c))
	queued := 0
	for _, e := range n.St.Outbox() {
		if e.Thread == c.Pub {
			queued++
		}
	}
	if queued == 0 {
		fmt.Println("Contact request delivered.")
	} else {
		fmt.Println("Contact request queued until they are reachable.")
	}
	fmt.Println("Run `spearhead verify " + c.Label + "` to compare safety codes.")
}

func cmdContacts(n *node.Node) {
	unread := map[string]int{}
	for _, m := range n.St.Unread() {
		unread[m.Thread]++
	}
	fmt.Printf("%-16s %-11s %-27s %-22s %-10s %s\n", "LABEL", "STATUS", "FINGERPRINT", "ADDRESS", "LAST SEEN", "UNREAD")
	for _, c := range n.St.ListContacts() {
		fmt.Printf("%-16s %-11s %-27s %-22s %-10s %d\n", clean(c.Label), mark(c), fp(c), c.LastAddr, ago(c.LastSeen), unread[c.Pub])
	}
}

func cmdRequests(n *node.Node) {
	all := n.St.ListContacts()
	k := 0
	for _, p := range all {
		if p.Status != store.StatusPending {
			continue
		}
		k++
		fmt.Printf("%s says their name is %q\n  fingerprint %s, first seen %s, %d message(s) waiting\n",
			p.Label, clean(p.TheirName), fp(p), ago(p.FirstSeen), len(n.St.Thread(p.Pub)))
		for _, m := range n.St.Thread(p.Pub) {
			body := strings.ReplaceAll(clean(m.Env.Body), "\n", " ")
			if len(body) > 70 {
				body = body[:70] + "..."
			}
			fmt.Printf("    > %s\n", body)
			break
		}
		for _, q := range all {
			if q.Status != store.StatusPending && (strings.EqualFold(p.TheirName, q.Label) || strings.EqualFold(p.TheirName, q.TheirName)) {
				fmt.Printf("  WARNING: same name as your contact %q but a DIFFERENT key. Could be a reinstall or an impersonator; verify before accepting.\n", q.Label)
			}
		}
		fmt.Printf("  -> `spearhead accept %s` or `spearhead decline %s`\n", fp(p), fp(p))
	}
	if k == 0 {
		fmt.Println("No pending requests.")
	}
}

func cmdResolve(n *node.Node, action string, rest []string) {
	if len(rest) != 1 {
		die("usage: spearhead %s <who>", action)
	}
	c := find(n, rest[0])
	switch action {
	case "accept":
		if c.Status == store.StatusBlocked {
			die("%s is blocked; use `spearhead unblock`", c.Label)
		}
		c.Status = store.StatusAccepted
	case "unblock":
		if c.Status != store.StatusBlocked {
			die("%s is not blocked", c.Label)
		}
		c.Status = store.StatusAccepted
	case "block":
		c.Status = store.StatusBlocked
	case "decline":
		if c.Status != store.StatusPending {
			die("only pending requests can be declined; use `block` for existing contacts")
		}
		check(n.St.DeleteContact(c.Pub))
		fmt.Println("Declined and removed", c.Label)
		return
	}
	check(n.St.SaveContact(c))
	fmt.Printf("%s is now %s\n", c.Label, c.Status)
	if action == "accept" {
		fmt.Printf("Run `spearhead verify %s` and compare safety codes with them.\n", c.Label)
	}
}

func cmdVerify(n *node.Node, rest []string) {
	if len(rest) == 0 {
		rest = []string{pickContact(n, "verify")}
	}
	if len(rest) != 1 {
		die("usage: spearhead verify [who]")
	}
	c := find(n, rest[0])
	pub, _ := identity.Decode(c.Pub)
	fmt.Printf("Safety code for you and %s:\n\n%s\n\n", c.Label, indent(identity.SafetyCode(n.ID.Pub, pub)))
	fmt.Println("Read it aloud or compare in person (not over the same chat). If both sides see exactly the same words, no one intercepted your introduction.")
	if !interactive() {
		die("run this in a terminal to confirm")
	}
	if confirm("Do the words match exactly?") {
		c.Verified = true
		check(n.St.SaveContact(c))
		fmt.Println(c.Label, "marked verified.")
	} else {
		fmt.Println("Not verified. If the words differ, do NOT trust this contact.")
	}
}

func indent(s string) string { return "    " + strings.ReplaceAll(s, "\n", "\n    ") }

func cmdSend(n *node.Node, rest []string) {
	fs := flag.NewFlagSet("send", flag.ContinueOnError)
	burn := fs.Bool("burn", false, "show once, then delete on both sides")
	ttl := fs.Int64("ttl", 0, "expire after this many seconds")
	force := fs.Bool("force", false, "skip the secret check")
	pos, tail := parseArgs(fs, rest)
	if len(pos) < 1 {
		pos = []string{pickContact(n, "send a message to")}
	}
	c := accepted(n, pos[0])
	body := strings.Join(append(pos[1:], tail...), " ")
	if body == "" && interactive() {
		fmt.Printf("Message to %s: ", c.Label)
		line, _ := stdin.ReadString('\n')
		body = strings.TrimSpace(line)
	}
	if body == "" && !interactive() {
		var buf bytes.Buffer
		buf.ReadFrom(stdin)
		body = strings.TrimSpace(buf.String())
	}
	if body == "" {
		die("nothing to send")
	}
	guardSecrets(n, body, *force)
	m, err := n.Send(c, body, node.SendOpts{TTL: *ttl, Burn: *burn})
	check(err)
	reportSend(m, c)
}

func cmdInbox(n *node.Node) {
	msgs := n.St.Unread()
	shown := 0
	for _, m := range msgs {
		c, err := n.St.GetContact(m.Thread)
		if err != nil {
			continue
		}
		if m.Env.Kind != protocol.KindRequest {
			render(n, c, m)
			shown++
		}
		if m.Env.Burn {
			n.St.DeleteMessage(m)
		} else {
			m.State = store.StateRead
			n.St.SaveMessage(m)
		}
	}
	if shown == 0 {
		fmt.Println("No new messages.")
	}
	if p := pendingCount(n); p > 0 {
		fmt.Printf("\n%d pending contact request(s): run `spearhead requests`\n", p)
	}
}

func pendingCount(n *node.Node) (k int) {
	for _, c := range n.St.ListContacts() {
		if c.Status == store.StatusPending {
			k++
		}
	}
	return
}

func visible(ms []*store.Message) (out []*store.Message) {
	for _, m := range ms {
		if !m.Env.Burn && m.Env.Kind != protocol.KindRequest {
			out = append(out, m)
		}
	}
	return
}

func parseSince(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	if strings.HasSuffix(s, "d") {
		d, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		return time.Now().AddDate(0, 0, -d), err
	}
	d, err := time.ParseDuration(s)
	return time.Now().Add(-d), err
}

func cmdLog(n *node.Node, rest []string) {
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	count := fs.Int("n", 30, "show the last N messages")
	grep := fs.String("grep", "", "only messages containing this text")
	since := fs.String("since", "", "only messages newer than this (e.g. 12h, 2d)")
	pos, _ := parseArgs(fs, rest)
	if len(pos) == 0 {
		pos = []string{pickContact(n, "see the history of")}
	}
	if len(pos) != 1 {
		die("usage: spearhead log [who] [--n N] [--grep TEXT] [--since 2d]")
	}
	c := find(n, pos[0])
	cut, err := parseSince(*since)
	check(err)
	var out []*store.Message
	for _, m := range visible(n.St.Thread(c.Pub)) {
		if time.Unix(m.Env.TS, 0).Before(cut) || (*grep != "" && !strings.Contains(strings.ToLower(m.Env.Body), strings.ToLower(*grep))) {
			continue
		}
		out = append(out, m)
	}
	if len(out) > *count {
		out = out[len(out)-*count:]
	}
	for _, m := range out {
		render(n, c, m)
	}
	if len(out) == 0 {
		fmt.Println("(no messages)")
	}
}

func cmdSearch(n *node.Node, rest []string) {
	if len(rest) == 0 {
		die("usage: spearhead search <term>")
	}
	term := strings.ToLower(strings.Join(rest, " "))
	k := 0
	for _, c := range n.St.ListContacts() {
		for _, m := range visible(n.St.Thread(c.Pub)) {
			if strings.Contains(strings.ToLower(m.Env.Body), term) {
				render(n, c, m)
				k++
			}
		}
	}
	if k == 0 {
		fmt.Println("(no matches)")
	}
}

func cmdChat(n *node.Node, rest []string) {
	if len(rest) == 0 {
		rest = []string{pickContact(n, "chat with")}
	}
	if len(rest) != 1 {
		die("usage: spearhead chat [who]")
	}
	c := accepted(n, rest[0])
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := n.Serve(ctx); err != nil {
		fmt.Println("(not listening here; if `spearhead listen` is running, incoming messages still appear)")
	} else {
		go n.Run2(ctx)
	}
	var seenMu sync.Mutex
	seen := map[string]bool{}
	hist := visible(n.St.Thread(c.Pub))
	if len(hist) > 15 {
		hist = hist[len(hist)-15:]
	}
	for _, m := range hist {
		render(n, c, m)
		seen[m.Env.ID] = true
	}
	fmt.Printf("--- chatting with %s (%s). Type /quit to leave. ---\n", c.Label, mark(c))
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
			for _, m := range visible(n.St.Thread(c.Pub)) {
				seenMu.Lock()
				already := seen[m.Env.ID]
				seen[m.Env.ID] = true
				seenMu.Unlock()
				if already {
					continue
				}
				if m.Dir == store.DirIn {
					render(n, c, m)
					m.State = store.StateRead
					n.St.SaveMessage(m)
				}
			}
		}
	}()
	sc := bufio.NewScanner(stdin)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "/quit" {
			return
		}
		if line == "" {
			continue
		}
		guardSecrets(n, line, false)
		m, err := n.Send(c, line, node.SendOpts{})
		if err != nil {
			fmt.Println("error:", err)
			continue
		}
		seenMu.Lock()
		seen[m.Env.ID] = true
		seenMu.Unlock()
		if m.State != store.StateDelivered {
			fmt.Println("  (" + m.State + ")")
		}
	}
}

func cmdListen(n *node.Node) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	n.OnMessage = func(c *store.Contact, m *store.Message) {
		if m.Env.Kind == protocol.KindRequest && c.Status == store.StatusPending {
			fmt.Printf("\a[request] %s (%s) wants to connect. Run `spearhead requests`.\n", clean(c.Label), fp(c))
			return
		}
		if c.Status != store.StatusAccepted {
			fmt.Printf("\a[request] message from unaccepted %s (%s); see `spearhead requests`.\n", clean(c.Label), fp(c))
			return
		}
		fmt.Print("\a")
		if m.Env.Burn {
			fmt.Printf("[burn message from %s] read it once with `spearhead inbox`.\n", clean(c.Label))
			return
		}
		render(n, c, m)
	}
	fmt.Printf("spearhead listening as %s (%s) on TCP %d", n.ID.DisplayName(), identity.Fingerprint(n.ID.Pub), n.Settings().ListenPort)
	if n.Settings().Discovery {
		fmt.Printf(", discovery on UDP %d", n.Settings().DiscoveryPort)
	}
	fmt.Println(". Ctrl+C to stop.")
	if err := n.Run(ctx); err != nil {
		die("cannot listen on port %d (already running?): %v", n.Settings().ListenPort, err)
	}
}

func cmdStatus(n *node.Node) {
	fmt.Printf("%s (%s), listen port %d\n", n.ID.DisplayName(), identity.Fingerprint(n.ID.Pub), n.Settings().ListenPort)
	ob := n.St.Outbox()
	fmt.Printf("Outbox: %d waiting\n", len(ob))
	for _, e := range ob {
		label := e.Thread
		if c, err := n.St.GetContact(e.Thread); err == nil {
			label = c.Label
		}
		next := "now"
		if e.NextTry > time.Now().Unix() {
			next = "in " + (time.Duration(e.NextTry-time.Now().Unix()) * time.Second).String()
		}
		fmt.Printf("  to %s: %d attempt(s), next retry %s\n", clean(label), e.Attempts, next)
	}
	failed := 0
	for _, c := range n.St.ListContacts() {
		for _, m := range n.St.Thread(c.Pub) {
			if m.Dir == store.DirOut && m.State == store.StateFailed {
				failed++
			}
		}
	}
	fmt.Printf("Failed messages: %d\nPending requests: %d\nNearby nodes seen (90s): ", failed, pendingCount(n))
	k := 0
	for _, p := range n.St.Peers() {
		if time.Since(time.Unix(p.Seen, 0)) < 90*time.Second {
			k++
		}
	}
	fmt.Println(k)
}
