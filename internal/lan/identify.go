package lan

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode"
)

func cleanText(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 160 {
		value = string(runes[:160])
	}
	return value
}
func localAddress(ip net.IP, interfaces []Interface) bool {
	if ip == nil || ip.IsLoopback() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, i := range interfaces {
		if !i.Up || i.Virtual {
			continue
		}
		for _, a := range i.Addresses {
			own, network, err := net.ParseCIDR(a)
			if err == nil && network.Contains(ip) && !own.Equal(ip) {
				return true
			}
		}
	}
	return false
}
func typeForServices(services []string) (string, string) {
	all := strings.ToLower(strings.Join(services, " "))
	for _, rule := range []struct {
		needles []string
		kind    string
	}{{[]string{"_ipp.", "_ipps.", "_printer.", "_pdl-datastream.", "9100/tcp", "631/tcp"}, "Printer"}, {[]string{"_googlecast.", "_airplay.", "_raop.", "_spotify-connect.", "_sonos.", "_roku-ecp.", "8009/tcp", "mediarenderer", "mediaserver"}, "Media device"}, {[]string{"internetgatewaydevice"}, "Router"}, {[]string{"_hap.", "hue bridge", "lighting", "_home-assistant."}, "Smart home device"}, {[]string{"_smb.", "445/tcp"}, "Computer or file server"}, {[]string{"_workstation.", "3389/tcp"}, "Computer"}, {[]string{"62078/tcp"}, "Phone or tablet"}} {
		for _, needle := range rule.needles {
			if strings.Contains(all, needle) {
				return rule.kind, needle
			}
		}
	}
	return "", ""
}
func applyGuess(d *Device) {
	if d.Kind == "" || d.Kind == "Network device" {
		if kind, evidence := typeForServices(d.Services); kind != "" {
			d.Kind = kind
			d.Confidence = "Likely"
			d.Identification = "Service evidence: " + evidence
		}
	}
	if d.Kind == "" || d.Kind == "Network device" {
		text := strings.ToLower(d.Hostname + " " + d.Name + " " + d.Model)
		for _, rule := range []struct{ needle, kind string }{{"synology", "Network storage"}, {"qnap", "Network storage"}, {"diskstation", "Network storage"}, {"raspberry", "Single-board computer"}, {"iphone", "Phone or tablet"}, {"ipad", "Phone or tablet"}, {"android", "Phone or tablet"}, {"macbook", "Computer"}, {"imac", "Computer"}, {"desktop", "Computer"}, {"laptop", "Computer"}, {"printer", "Printer"}, {"chromecast", "Media device"}, {"apple-tv", "Media device"}, {"roku", "Media device"}, {"smart-tv", "Media device"}} {
			if strings.Contains(text, rule.needle) {
				d.Kind = rule.kind
				d.Confidence = "Likely"
				d.Identification = "Name/model hint: " + rule.needle
				break
			}
		}
	}
	if (d.Name == "" || net.ParseIP(d.Name) != nil || d.Name == d.Kind) && d.Hostname != "" {
		d.Name = d.Hostname
	}
}
func avahiDecode(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+3 < len(value) {
			if n, err := strconv.Atoi(value[i+1 : i+4]); err == nil && n >= 0 && n <= 255 {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(value[i])
	}
	return cleanText(b.String())
}

var avahiTXT = regexp.MustCompile(`"([^"\r\n]*)"`)

func parseAvahi(raw []byte, interfaces []Interface) []Device {
	result := []Device{}
	for _, line := range strings.Split(string(raw), "\n") {
		parts := strings.Split(line, ";")
		if len(parts) < 10 || parts[0] != "=" {
			continue
		}
		ip := net.ParseIP(cleanIP(parts[7]))
		if !localAddress(ip, interfaces) {
			continue
		}
		d := Device{IP: ip.String(), Interface: avahiDecode(parts[1]), Name: avahiDecode(parts[3]), Hostname: strings.TrimSuffix(avahiDecode(parts[6]), "."), Services: []string{avahiDecode(parts[4]) + " · port " + parts[8]}, Source: "mDNS / DNS-SD", Status: "Advertised on LAN", Seen: time.Now().Unix()}
		if kind, evidence := typeForServices(d.Services); kind != "" {
			d.Kind = kind
			d.Confidence = "Advertised"
			d.Identification = "Advertised service: " + evidence
		}
		for _, match := range avahiTXT.FindAllStringSubmatch(strings.Join(parts[9:], ";"), 32) {
			key, value, ok := strings.Cut(avahiDecode(match[1]), "=")
			if !ok {
				continue
			}
			switch strings.ToLower(key) {
			case "ty", "model", "md", "product":
				if d.Model == "" {
					d.Model = cleanText(strings.Trim(value, "()"))
				}
			case "manufacturer", "vendor":
				d.Manufacturer = cleanText(value)
			}
		}
		result = append(result, d)
	}
	return result
}
func multicastDescriptions(ctx context.Context, interfaces []Interface) []Device {
	addresses := []net.IP{}
	for _, i := range interfaces {
		if !i.Up || i.Virtual {
			continue
		}
		for _, a := range i.Addresses {
			ip, _, err := net.ParseCIDR(a)
			if err == nil && ip.To4() != nil && (ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
				addresses = append(addresses, ip)
				break
			}
		}
		if len(addresses) == 4 {
			break
		}
	}
	result := []Device{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, local := range addresses {
		wg.Add(1)
		go func(local net.IP) {
			defer wg.Done()
			conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: local})
			if err != nil {
				return
			}
			defer conn.Close()
			conn.SetDeadline(time.Now().Add(2 * time.Second))
			query := "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: upnp:rootdevice\r\n\r\n"
			conn.WriteToUDP([]byte(query), &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900})
			packet := make([]byte, 8192)
			seen := map[string]bool{}
			for attempts := 0; attempts < 64; attempts++ {
				n, sender, err := conn.ReadFromUDP(packet)
				if err != nil {
					return
				}
				if !localAddress(sender.IP, interfaces) || seen[sender.IP.String()] {
					continue
				}
				location := ""
				for _, line := range strings.Split(string(packet[:n]), "\n") {
					key, value, ok := strings.Cut(line, ":")
					if ok && strings.EqualFold(strings.TrimSpace(key), "location") {
						location = strings.TrimSpace(value)
					}
				}
				if !descriptionURL(location, sender.IP) {
					continue
				}
				seen[sender.IP.String()] = true
				device, err := readDescription(ctx, location, sender.IP)
				if err != nil {
					continue
				}
				mu.Lock()
				result = append(result, device)
				mu.Unlock()
			}
		}(local)
	}
	wg.Wait()
	return result
}
func descriptionURL(location string, sender net.IP) bool {
	u, err := url.Parse(location)
	if err != nil || u.Scheme != "http" || u.User != nil || u.Fragment != "" || sender == nil || sender.IsLoopback() || !net.ParseIP(u.Hostname()).Equal(sender) {
		return false
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return false
		}
	}
	return len(location) <= 2048
}
func readDescription(ctx context.Context, location string, sender net.IP) (Device, error) {
	request, err := http.NewRequestWithContext(ctx, "GET", location, nil)
	if err != nil {
		return Device{}, err
	}
	transport := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 600 * time.Millisecond}).DialContext, DisableKeepAlives: true, MaxResponseHeaderBytes: 16 << 10}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 1500 * time.Millisecond, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request)
	if err != nil {
		return Device{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return Device{}, fmt.Errorf("description status %d", response.StatusCode)
	}
	return parseDescription(io.LimitReader(response.Body, 256<<10), sender.String())
}
func parseDescription(reader io.Reader, ip string) (Device, error) {
	var document struct {
		Device struct {
			Type         string `xml:"deviceType"`
			Name         string `xml:"friendlyName"`
			Manufacturer string `xml:"manufacturer"`
			Model        string `xml:"modelName"`
			UUID         string `xml:"UDN"`
		} `xml:"device"`
	}
	if err := xml.NewDecoder(reader).Decode(&document); err != nil {
		return Device{}, err
	}
	root := document.Device
	if root.Type == "" && root.Name == "" {
		return Device{}, fmt.Errorf("no UPnP device description")
	}
	d := Device{IP: ip, Name: cleanText(root.Name), Manufacturer: cleanText(root.Manufacturer), Model: cleanText(root.Model), UUID: cleanText(root.UUID), Source: "UPnP / SSDP", Status: "Responding", Seen: time.Now().Unix(), Services: []string{cleanText(root.Type)}}
	if kind, evidence := typeForServices(d.Services); kind != "" {
		d.Kind = kind
		d.Confidence = "Advertised"
		d.Identification = "UPnP device type: " + evidence
	}
	return d, nil
}
func probeAddress(d Device) string {
	ip := net.ParseIP(d.IP)
	if ip != nil && ip.IsLinkLocalUnicast() && len(d.Interfaces) > 0 {
		return d.IP + "%" + d.Interfaces[0]
	}
	return d.IP
}

func probeDevice(ctx context.Context, d Device, interfaces []Interface) Device {
	ip := net.ParseIP(d.IP)
	if !localAddress(ip, interfaces) {
		return d
	}
	lookup, cancel := context.WithTimeout(ctx, 550*time.Millisecond)
	names, _ := net.DefaultResolver.LookupAddr(lookup, d.IP)
	cancel()
	if len(names) > 0 {
		d.Hostname = cleanText(strings.TrimSuffix(names[0], "."))
		d.Sources = distinct(d.Sources, []string{"Reverse DNS"})
	}
	ports := []struct {
		port int
		name string
	}{{631, "Printing"}, {9100, "Printing"}, {445, "File sharing"}, {3389, "Remote desktop"}, {62078, "Apple mobile service"}, {8009, "Cast"}, {22, "SSH"}, {80, "Web interface"}, {443, "Secure web interface"}}
	for _, p := range ports {
		if ctx.Err() != nil {
			break
		}
		connection, err := (&net.Dialer{Timeout: 100 * time.Millisecond}).DialContext(ctx, "tcp", net.JoinHostPort(probeAddress(d), strconv.Itoa(p.port)))
		if errors.Is(err, syscall.ECONNREFUSED) {
			d.Status = "Responding"
			d.Seen = time.Now().Unix()
		}
		if err == nil {
			connection.Close()
			d.Services = distinct(d.Services, []string{fmt.Sprintf("%d/tcp · %s", p.port, p.name)})
			d.Status = "Responding"
			d.Seen = time.Now().Unix()
			d.Sources = distinct(d.Sources, []string{"Local service probes"})
		}
	}
	applyGuess(&d)
	return d
}
func (m *Monitor) identify(snapshot Snapshot, scanned bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	discoveries := make(chan []Device, 2)
	go func() {
		commandCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		discoveries <- parseAvahi(command(commandCtx, "avahi-browse", "-a", "-r", "-p", "-t"), snapshot.Interfaces)
	}()
	go func() { discoveries <- multicastDescriptions(ctx, snapshot.Interfaces) }()
	devices := Inventory(snapshot, nil, "", "")
	vendors := vendorsFor(devices)
	jobs := make(chan Device)
	found := make(chan Device, len(devices))
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for d := range jobs {
				d.Vendor = vendors[d.MAC]
				found <- probeDevice(ctx, d, snapshot.Interfaces)
			}
		}()
	}
	for _, d := range devices {
		if d.Self {
			continue
		}
		if ctx.Err() != nil {
			break
		}
		jobs <- d
	}
	close(jobs)
	workers.Wait()
	close(found)
	hints := []Device{}
	for d := range found {
		hints = append(hints, d)
	}
	for i := 0; i < 2; i++ {
		select {
		case discovered := <-discoveries:
			hints = append(hints, discovered...)
		case <-ctx.Done():
		}
	}
	fresh := readSnapshot()
	owners := vendorsFor(fresh.Devices)
	for _, neighbor := range fresh.Devices {
		if vendor := owners[validMAC(neighbor.MAC)]; vendor != "" {
			hints = append(hints, Device{IP: neighbor.IP, MAC: neighbor.MAC, Interface: neighbor.Interface, Vendor: vendor, Source: "Offline IEEE MAC assignment"})
		}
	}
	// Enrichment starts with current observations but does not keep stale neighbor
	// rows forever. Discovery metadata expires when the next identification runs.
	m.mu.Lock()
	m.hints = hints
	m.identified = time.Now()
	fresh.LastScan = m.cached.LastScan
	fresh.ScanTargets = m.cached.ScanTargets
	if scanned {
		fresh.LastScan = time.Now().Unix()
		fresh.ScanTargets = snapshot.ScanTargets
	}
	m.cached = fresh
	m.updated = time.Now()
	m.mu.Unlock()
}
