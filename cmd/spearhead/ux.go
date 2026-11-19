package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"spearhead/internal/api"
	"spearhead/internal/config"
	"spearhead/internal/desktop"
	"spearhead/internal/identity"
	"spearhead/internal/node"
	"spearhead/internal/store"
)

// pauseOnExit keeps the console window open on errors when SpearHead was started
// by double-clicking, so people can read what went wrong.
var pauseOnExit bool

func pause() {
	if pauseOnExit && interactive() {
		fmt.Print("\nPress Enter to close this window...")
		stdin.ReadString('\n')
	}
}

var commands = []string{"init", "whoami", "invite", "nearby", "add", "contacts", "requests", "accept", "decline", "block",
	"unblock", "verify", "send", "inbox", "chat", "log", "search", "listen", "status", "doctor", "app", "help"}

var helpText = map[string]string{
	"app":      "spearhead app [--background]\n  Opens the native Qt Quick desktop. Closing its window keeps it running in the tray. --background starts hidden.",
	"init":     "spearhead init [--name NAME] [--port PORT]\n  Creates your identity. Plain `spearhead` does this for you with a friendly prompt.",
	"add":      "spearhead add <invite-code | host:port | nearby-name> [label]\n  Adds a person. Use `spearhead nearby` to see who is on your network.\n  Example: spearhead add dave\n  Example: spearhead add rkt1:eyJr... \"Dave from work\"",
	"send":     "spearhead send [--burn] [--ttl SECONDS] [--force] [who] [message]\n  Sends a message. Leave out the name to pick from a list, and the message to be asked for it.\n  --burn   read once, then deleted on both sides\n  Example: spearhead send dave \"Printer at Acme fixed\"",
	"verify":   "spearhead verify [who]\n  Shows 8 safety words. Compare them out loud with that person. Same words = nobody in the middle.",
	"inbox":    "spearhead inbox\n  Shows new messages and marks them read. Read-once messages are deleted after showing.",
	"chat":     "spearhead chat [who]\n  Live conversation in the terminal. Type /quit to leave.",
	"log":      "spearhead log [who] [--n 30] [--grep TEXT] [--since 2d]\n  Shows saved conversation history.",
	"search":   "spearhead search <text>\n  Searches every saved conversation.",
	"nearby":   "spearhead nearby\n  Lists SpearHead users seen on your network in the last 90 seconds.",
	"requests": "spearhead requests\n  Shows people who want to connect. Then: spearhead accept <who> or spearhead decline <who>.",
	"listen":   "spearhead listen\n  Runs SpearHead without the desktop (headless), printing new messages here.",
	"doctor":   "spearhead doctor\n  Checks your setup and explains how to fix problems (firewall, ports, discovery).",
	"invite":   "spearhead invite\n  Prints your invite code. Send it to someone so they can add you from anywhere.",
	"status":   "spearhead status\n  Shows queued messages and connection basics.",
}

func cmdHelp(rest []string) {
	if len(rest) == 1 {
		if t, ok := helpText[rest[0]]; ok {
			fmt.Println(t)
			return
		}
		fmt.Printf("No help for %q. ", rest[0])
	}
	fmt.Println(usage)
	fmt.Println("\nMost people only need plain `spearhead` (it opens the app). Try `spearhead help <command>` for details and examples.")
}

func lev(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

func suggest(cmd string) string {
	best, bd := "", 3
	for _, c := range commands {
		if d := lev(strings.ToLower(cmd), c); d < bd {
			best, bd = c, d
		}
	}
	return best
}

// pickContact lets a person choose from a numbered list when they left out <who>.
func pickContact(n *node.Node, verb string) string {
	var list []*store.Contact
	for _, c := range n.St.ListContacts() {
		if c.Status == store.StatusAccepted {
			list = append(list, c)
		}
	}
	if len(list) == 0 {
		die("you don't have any contacts yet. Run `spearhead` for the desktop app, or `spearhead nearby` then `spearhead add <name>`.")
	}
	if len(list) == 1 {
		return list[0].Label
	}
	if !interactive() {
		die("usage: spearhead %s <who>", verb)
	}
	fmt.Printf("Who do you want to %s?\n", verb)
	for i, c := range list {
		fmt.Printf("  %d) %s\n", i+1, c.Label)
	}
	fmt.Print("Number: ")
	line, _ := stdin.ReadString('\n')
	i, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || i < 1 || i > len(list) {
		die("that wasn't one of the numbers listed")
	}
	return list[i-1].Label
}

type lockInfo struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
}

func lockPath(home string) string { return filepath.Join(home, "ui.json") }

func runningUI(home string) (string, bool) {
	b, err := os.ReadFile(lockPath(home))
	if err != nil {
		return "", false
	}
	var li lockInfo
	if json.Unmarshal(b, &li) != nil || li.Port == 0 {
		return "", false
	}
	c := http.Client{Timeout: 800 * time.Millisecond}
	r, err := c.Get(fmt.Sprintf("http://127.0.0.1:%d/api/ping", li.Port))
	if err != nil {
		return "", false
	}
	defer r.Body.Close()
	buf, err := io.ReadAll(io.LimitReader(r.Body, 32))
	if err != nil || (string(buf) != "spearhead" && string(buf) != "racket") {
		return "", false
	}
	return fmt.Sprintf("http://127.0.0.1:%d/?t=%s", li.Port, li.Token), true
}

// cmdApp runs the peer service and authenticated Qt Quick desktop.
func cmdApp(home string, args ...string) {
	background := false
	for _, arg := range args {
		if arg == "--background" {
			background = true
		} else {
			die("unknown app option %q", arg)
		}
	}
	if desktop.Helper() == "" {
		die("the native desktop helper is missing. Use the AppImage or install the complete package; use `spearhead listen` for headless operation")
	}
	pauseOnExit = true
	if !identity.Exists(home) {
		name := os.Getenv("USER")
		if name == "" {
			name = "SpearHead user"
		}
		_, err := identity.Create(home, name)
		check(err)
		check(config.Save(home, config.Default()))
	}
	if url, ok := runningUI(home); ok {
		fmt.Println("SpearHead is already running. Reopening it.")
		child, err := desktop.Start(context.Background(), url, false)
		check(err)
		check(child.Wait())
		return
	}
	n, err := node.New(home)
	check(err)
	var logged atomic.Int32
	n.Logf = func(format string, args ...any) {
		if logged.Add(1) <= 20 {
			fmt.Fprintf(os.Stderr, time.Now().Format("15:04:05 ")+format+"\n", args...)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := api.New(n)
	srv.OnQuit = stop
	if err := n.Serve(ctx); err != nil {
		fmt.Printf("Peer port %d is busy: %v. Run spearhead doctor for help.\n", n.Settings().ListenPort, err)
	} else {
		go n.Run2(ctx)
	}
	url, err := srv.Serve(ctx, n.Settings().UIPort)
	check(err)
	lock, _ := json.Marshal(lockInfo{Port: srv.Port, Token: srv.Token})
	check(os.WriteFile(lockPath(home), lock, 0o600))
	defer os.Remove(lockPath(home))
	fmt.Printf("SpearHead is running as %s. Choose the window-close behavior in Settings.\n", n.ID.DisplayName())
	child, err := desktop.Start(ctx, url, background)
	check(err)
	done := make(chan struct{})
	go func() {
		defer close(done)
		err := child.Wait()
		if ctx.Err() == nil {
			if err != nil {
				fmt.Fprintln(os.Stderr, "Native desktop stopped:", err)
			}
			stop()
		}
	}()
	<-ctx.Done()
	<-done
}

func cmdDoctor(home string) {
	pass := func(ok bool, what, fix string) {
		if ok {
			fmt.Printf("[ OK ] %s\n", what)
		} else {
			fmt.Printf("[FAIL] %s\n       Fix: %s\n", what, fix)
		}
	}
	info := func(f string, a ...any) { fmt.Printf("[INFO] "+f+"\n", a...) }
	fmt.Println("SpearHead setup check")
	fmt.Println("------------------")
	pass(identity.Exists(home), "You are set up (identity found)", "run `spearhead` and follow the prompts")
	if !identity.Exists(home) {
		return
	}
	cfg, err := config.Load(home)
	check(err)
	probe := filepath.Join(home, ".doctor")
	pass(os.WriteFile(probe, []byte("x"), 0o600) == nil, "SpearHead can save data in "+home, "make sure that folder is writable")
	os.Remove(probe)

	_, running := runningUI(home)
	if running {
		info("SpearHead is already running on this computer, so its ports being busy is expected.")
	} else {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", cfg.ListenPort))
		pass(err == nil, fmt.Sprintf("Port %d is free for receiving messages", cfg.ListenPort),
			fmt.Sprintf("another program is using it. Close that program, or edit listen_port in %s (both people can use different ports).", filepath.Join(home, "config.json")))
		if ln != nil {
			ln.Close()
		}
		uc, err := net.ListenUDP("udp4", &net.UDPAddr{Port: cfg.DiscoveryPort})
		pass(err == nil, fmt.Sprintf("Port %d is free for finding nearby people", cfg.DiscoveryPort), "another program is using it; close it or change discovery_port in config.json")
		if uc != nil {
			uc.Close()
		}
	}
	addrs := node.LocalAddrs(cfg.ListenPort)
	pass(len(addrs) > 0, "This computer has a network address", "connect to your office network or VPN")
	for _, a := range addrs {
		info("Others on your network could reach you at %s", a)
	}
	st, err := store.Open(home)
	check(err)
	seen := 0
	for _, p := range st.Peers() {
		if time.Since(time.Unix(p.Seen, 0)) < 2*time.Minute {
			seen++
		}
	}
	if seen > 0 {
		info("%d other SpearHead user(s) seen on the network in the last 2 minutes.", seen)
	} else {
		info("No other SpearHead users seen recently. They need SpearHead open on the same network. Different subnets or VLANs? Use an invite code instead.")
	}
	if runtime.GOOS == "windows" {
		exe, _ := os.Executable()
		fmt.Println("\nWindows Firewall: if people can't reach you, allow SpearHead on private networks. In an Administrator PowerShell:")
		fmt.Printf("  netsh advfirewall firewall add rule name=\"SpearHead\" dir=in action=allow program=\"%s\" profile=private\n", exe)
	}
	cs := st.ListContacts()
	sort.Slice(cs, func(i, j int) bool { return cs[i].Label < cs[j].Label })
	unv := 0
	for _, c := range cs {
		if c.Status == store.StatusAccepted && !c.Verified {
			unv++
		}
	}
	if unv > 0 {
		info("%d contact(s) not verified yet. Open the person in the app and click Verify (10 seconds).", unv)
	}
}
