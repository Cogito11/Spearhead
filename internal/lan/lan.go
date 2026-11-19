// Package lan reports local interfaces and reachable neighbor devices. Inventory
// is observational: offline hosts and isolated Wi-Fi clients cannot be inferred.
package lan

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type Interface struct {
	Name         string   `json:"name"`
	MAC          string   `json:"mac"`
	MTU          int      `json:"mtu"`
	Up           bool     `json:"up"`
	Virtual      bool     `json:"virtual"`
	DefaultRoute bool     `json:"default_route"`
	Addresses    []string `json:"addresses"`
}

type Device struct {
	ID             string   `json:"id"`
	IP             string   `json:"ip"`
	MAC            string   `json:"mac"`
	Interface      string   `json:"interface"`
	Name           string   `json:"name"`
	Status         string   `json:"status"`
	Source         string   `json:"source"`
	Seen           int64    `json:"seen"`
	Pub            string   `json:"pub,omitempty"`
	Platform       string   `json:"platform,omitempty"`
	BroadcastName  string   `json:"broadcast_name,omitempty"`
	Address        string   `json:"address,omitempty"`
	Self           bool     `json:"self"`
	Addresses      []string `json:"addresses"`
	MACs           []string `json:"macs"`
	Interfaces     []string `json:"interfaces"`
	Sources        []string `json:"sources"`
	Aliases        []string `json:"aliases"`
	Services       []string `json:"services"`
	Hostname       string   `json:"hostname,omitempty"`
	Vendor         string   `json:"vendor,omitempty"`
	Manufacturer   string   `json:"manufacturer,omitempty"`
	Model          string   `json:"model,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Confidence     string   `json:"confidence,omitempty"`
	Identification string   `json:"identification,omitempty"`
	UUID           string   `json:"uuid,omitempty"`
}

type Snapshot struct {
	Hostname       string      `json:"hostname"`
	Platform       string      `json:"platform"`
	Interfaces     []Interface `json:"interfaces"`
	Devices        []Device    `json:"devices"`
	Gateway        string      `json:"gateway"`
	DNS            []string    `json:"dns"`
	Identifying    bool        `json:"identifying"`
	Scanning       bool        `json:"scanning"`
	LastIdentified int64       `json:"last_identified"`
	LastScan       int64       `json:"last_scan"`
	ScanTargets    int         `json:"scan_targets"`
	Coverage       string      `json:"coverage"`
	Error          string      `json:"error,omitempty"`
}

type Monitor struct {
	mu         sync.Mutex
	cached     Snapshot
	updated    time.Time
	hints      []Device
	identified time.Time
	automatic  bool
}

func New() *Monitor { return &Monitor{automatic: true} }

// NewPassive reads OS observations without initiating identification probes.
func NewPassive() *Monitor { return &Monitor{} }

func command(ctx context.Context, name string, args ...string) []byte {
	b, _ := exec.CommandContext(ctx, name, args...).Output()
	return b
}

func parseNeighbors(raw []byte, now int64) []Device {
	var rows []struct {
		Dst    string
		Dev    string
		Lladdr string
		State  []string
	}
	json.Unmarshal(raw, &rows)
	out := []Device{}
	for _, r := range rows {
		ip := net.ParseIP(r.Dst)
		if ip == nil || ip.IsLoopback() || ip.IsMulticast() || validMAC(r.Lladdr) == "" {
			continue
		}
		status := strings.Join(r.State, ", ")
		if strings.Contains(status, "FAILED") || strings.Contains(status, "INCOMPLETE") {
			continue
		}
		observed := int64(0)
		if strings.Contains(status, "REACHABLE") || strings.Contains(status, "PERMANENT") {
			observed = now
		}
		friendly := map[string]string{"REACHABLE": "Reachable", "STALE": "Previously seen", "DELAY": "Checking reachability", "PROBE": "Checking reachability", "PERMANENT": "Known neighbor"}[status]
		if friendly != "" {
			status = friendly
		}
		out = append(out, Device{ID: r.Dev + "/" + r.Dst, IP: r.Dst, MAC: r.Lladdr, Interface: r.Dev, Status: status, Source: "Neighbor table", Seen: observed})
	}
	return out
}

func readSnapshot() Snapshot {
	s := Snapshot{Interfaces: []Interface{}, Devices: []Device{}, DNS: []string{}, Platform: runtime.GOOS + "/" + runtime.GOARCH,
		Coverage: "One entry per observed device. Names and types use local advertisements and best-effort hints; offline or isolated devices may not appear."}
	s.Hostname, _ = os.Hostname()
	ifs, err := net.Interfaces()
	if err != nil {
		s.Error = err.Error()
		return s
	}
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		i := Interface{Name: ifc.Name, MAC: ifc.HardwareAddr.String(), MTU: ifc.MTU, Up: ifc.Flags&net.FlagUp != 0, Addresses: []string{}}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			i.Addresses = append(i.Addresses, a.String())
		}
		if runtime.GOOS == "linux" {
			_, physicalError := os.Stat("/sys/class/net/" + ifc.Name + "/device")
			i.Virtual = physicalError != nil
		}
		s.Interfaces = append(s.Interfaces, i)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if runtime.GOOS == "linux" {
		s.Devices = parseNeighbors(command(ctx, "ip", "-j", "neigh", "show"), time.Now().Unix())
		var routes []struct {
			Dst     string
			Gateway string
			Dev     string
		}
		json.Unmarshal(command(ctx, "ip", "-j", "route", "show", "default"), &routes)
		if len(routes) > 0 {
			s.Gateway = routes[0].Gateway
			for i := range s.Interfaces {
				s.Interfaces[i].DefaultRoute = s.Interfaces[i].Name == routes[0].Dev
			}
			if s.Gateway != "" {
				s.Gateway += " (" + routes[0].Dev + ")"
			}
		}
	}
	b, _ := os.ReadFile("/etc/resolv.conf")
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "nameserver" && net.ParseIP(f[1]) != nil {
			s.DNS = append(s.DNS, f[1])
		}
	}
	return s
}

func (m *Monitor) Snapshot() Snapshot {
	m.mu.Lock()
	if time.Since(m.updated) > 15*time.Second {
		fresh := readSnapshot()
		fresh.Scanning, fresh.Identifying, fresh.LastScan, fresh.ScanTargets = m.cached.Scanning, m.cached.Identifying, m.cached.LastScan, m.cached.ScanTargets
		m.cached = fresh
		m.updated = time.Now()
	}
	snapshot := m.withHints(m.cached)
	if !m.identified.IsZero() {
		snapshot.LastIdentified = m.identified.Unix()
	}
	identify := m.automatic && !m.cached.Scanning && !m.cached.Identifying && time.Since(m.identified) > 90*time.Second
	initialScan := identify && m.cached.LastScan == 0 && len(scanTargets(snapshot.Interfaces)) > 0
	if identify {
		m.cached.Identifying = true
		snapshot.Identifying = true
		if initialScan {
			m.cached.Scanning = true
			snapshot.Scanning = true
			m.cached.ScanTargets = len(scanTargets(snapshot.Interfaces))
			snapshot.ScanTargets = m.cached.ScanTargets
		}
	}
	m.mu.Unlock()
	if identify {
		if initialScan {
			go m.scan(snapshot)
		} else {
			go m.identify(snapshot, false)
		}
	}
	return snapshot
}
func (m *Monitor) withHints(snapshot Snapshot) Snapshot {
	snapshot.Devices = append([]Device{}, snapshot.Devices...)
	activeIPs, activeMACs := map[string]bool{}, map[string]bool{}
	for _, d := range snapshot.Devices {
		activeIPs[cleanIP(d.IP)] = true
		if mac := validMAC(d.MAC); mac != "" {
			activeMACs[mac] = true
		}
	}
	now := time.Now().Unix()
	// Metadata enriches existing observations. Only recent service announcements
	// can introduce an extra device; obsolete probe rows do not become ghosts.
	for _, hint := range m.hints {
		announced := (strings.Contains(hint.Source, "mDNS") || strings.Contains(hint.Source, "SSDP")) && hint.Seen > now-120
		if activeIPs[cleanIP(hint.IP)] || activeMACs[validMAC(hint.MAC)] || announced {
			snapshot.Devices = append(snapshot.Devices, hint)
		}
	}
	return snapshot
}

// scanTargets bounds work to four private, directly connected /24 segments.
// A narrower configured network is respected. No Internet or routed subnet scan.
func scanTargets(ifs []Interface) []string {
	out := []string{}
	seen := map[string]bool{}
	self := map[string]bool{}
	for _, ifc := range ifs {
		for _, address := range ifc.Addresses {
			ip, _, err := net.ParseCIDR(address)
			if err == nil {
				self[ip.String()] = true
			}
		}
	}
	for _, ifc := range ifs {
		if !ifc.Up || ifc.Virtual {
			continue
		}
		for _, addr := range ifc.Addresses {
			ip, network, err := net.ParseCIDR(addr)
			if err != nil || ip.To4() == nil || !(ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
				continue
			}
			ones, bits := network.Mask.Size()
			if bits != 32 || ones >= 31 {
				continue
			}
			if ones < 24 {
				network = &net.IPNet{IP: ip.Mask(net.CIDRMask(24, 32)), Mask: net.CIDRMask(24, 32)}
			}
			base := network.IP.To4()
			ones, _ = network.Mask.Size()
			count := 1 << uint(32-ones)
			n := uint32(base[0])<<24 | uint32(base[1])<<16 | uint32(base[2])<<8 | uint32(base[3])
			for host := 1; host < count-1 && len(out) < 1024; host++ {
				x := n + uint32(host)
				target := fmt.Sprintf("%d.%d.%d.%d", byte(x>>24), byte(x>>16), byte(x>>8), byte(x))
				if !self[target] && !seen[target] {
					seen[target] = true
					out = append(out, target)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

func (m *Monitor) StartScan() bool {
	m.mu.Lock()
	if m.cached.Scanning || m.cached.Identifying {
		m.mu.Unlock()
		return false
	}
	if time.Since(m.updated) > 15*time.Second {
		fresh := readSnapshot()
		fresh.LastScan = m.cached.LastScan
		fresh.ScanTargets = m.cached.ScanTargets
		m.cached = fresh
		m.updated = time.Now()
	}
	m.cached.Scanning = true
	m.cached.Identifying = true
	m.cached.ScanTargets = len(scanTargets(m.cached.Interfaces))
	snapshot := m.cached
	m.mu.Unlock()
	go m.scan(snapshot)
	return true
}
func (m *Monitor) scan(snapshot Snapshot) {
	targets := scanTargets(snapshot.Interfaces)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	jobs := make(chan string)
	var workers sync.WaitGroup
	for i := 0; i < 32; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for ip := range jobs {
				connection, err := (&net.Dialer{Timeout: 200 * time.Millisecond}).DialContext(ctx, "tcp", net.JoinHostPort(ip, "9"))
				if err == nil {
					connection.Close()
				}
			}
		}()
	}
loop:
	for _, target := range targets {
		select {
		case jobs <- target:
		case <-ctx.Done():
			break loop
		}
	}
	close(jobs)
	workers.Wait()
	snapshot = readSnapshot()
	snapshot.ScanTargets = len(targets)
	m.identify(snapshot, true)
}
