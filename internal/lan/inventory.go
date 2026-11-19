package lan

import (
	"net"
	"sort"
	"strings"
	"time"
)

func cleanIP(value string) string {
	value = strings.Trim(strings.TrimSpace(value), "[]")
	if at := strings.IndexByte(value, '%'); at >= 0 {
		value = value[:at]
	}
	if ip := net.ParseIP(value); ip != nil {
		return ip.String()
	}
	return value
}
func validMAC(value string) string {
	mac, err := net.ParseMAC(value)
	if err != nil || len(mac) != 6 || mac[0]&1 != 0 {
		return ""
	}
	nonzero := false
	for _, b := range mac {
		nonzero = nonzero || b != 0
	}
	if !nonzero {
		return ""
	}
	return mac.String()
}
func distinct(values ...[]string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, list := range values {
		for _, value := range list {
			if value != "" && !seen[value] {
				seen[value] = true
				out = append(out, value)
			}
		}
	}
	sort.Strings(out)
	return out
}
func addressRank(address string) int {
	ip := net.ParseIP(cleanIP(address))
	if ip == nil {
		return 4
	}
	if ip.IsLoopback() {
		return 5
	}
	if ip.To4() != nil {
		return 0
	}
	if ip.IsLinkLocalUnicast() {
		return 3
	}
	return 1
}
func orderAddresses(addresses []string) []string {
	addresses = distinct(addresses)
	sort.SliceStable(addresses, func(i, j int) bool {
		if addressRank(addresses[i]) != addressRank(addresses[j]) {
			return addressRank(addresses[i]) < addressRank(addresses[j])
		}
		return addresses[i] < addresses[j]
	})
	return addresses
}
func overlaps(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x != "" && x == y {
				return true
			}
		}
	}
	return false
}
func canonical(d Device) Device {
	d.IP = cleanIP(d.IP)
	d.MAC = validMAC(d.MAC)
	ips := []string{}
	for _, a := range append(append([]string{}, d.Addresses...), d.IP) {
		if a != "" {
			ips = append(ips, cleanIP(a))
		}
	}
	d.Addresses = orderAddresses(ips)
	macs := []string{}
	for _, a := range append(append([]string{}, d.MACs...), d.MAC) {
		if a = validMAC(a); a != "" {
			macs = append(macs, a)
		}
	}
	d.MACs = distinct(macs)
	d.Interfaces = distinct(d.Interfaces, []string{d.Interface})
	sources := []string{}
	for _, value := range append(append([]string{}, d.Sources...), d.Source) {
		sources = append(sources, strings.Split(value, ", ")...)
	}
	d.Sources = distinct(sources)
	d.Aliases = distinct(d.Aliases, []string{d.ID})
	for _, ip := range d.Addresses {
		d.Aliases = distinct(d.Aliases, []string{"ip/" + ip})
	}
	d.Services = distinct(d.Services)
	if len(d.Addresses) > 0 {
		d.IP = d.Addresses[0]
	}
	if d.MAC == "" && len(d.MACs) > 0 {
		d.MAC = d.MACs[0]
	}
	return d
}
func sameDevice(a, b Device, gateway string, gatewayMACs []string) bool {
	if a.Pub != "" && b.Pub != "" && a.Pub != b.Pub {
		return false
	}
	if a.Pub != "" && a.Pub == b.Pub || a.UUID != "" && a.UUID == b.UUID {
		return true
	}
	for _, ip := range a.Addresses {
		for _, other := range b.Addresses {
			if ip == other {
				parsed := net.ParseIP(ip)
				if parsed == nil || !parsed.IsLinkLocalUnicast() || overlaps(a.Interfaces, b.Interfaces) {
					return true
				}
			}
		}
	}
	if !overlaps(a.MACs, b.MACs) {
		return false
	}
	// A router may proxy ARP for other IPv4 hosts; its MAC is not proof that those
	// IPs belong to the router. IPv6 aliases on its own interface still combine.
	if gateway != "" && (overlaps(a.Addresses, []string{gateway}) || overlaps(b.Addresses, []string{gateway}) || overlaps(a.MACs, gatewayMACs)) {
		for _, first := range a.Addresses {
			for _, second := range b.Addresses {
				x, y := net.ParseIP(first), net.ParseIP(second)
				if x != nil && y != nil && x.To4() != nil && y.To4() != nil && first != second {
					return false
				}
			}
		}
	}
	return true
}
func mergeDevice(a, b Device) Device {
	if (a.Name == "" || a.Name == a.IP || a.Name == a.Kind) && (b.Name != "" && b.Name != b.IP && b.Name != b.Kind) {
		a.Name = b.Name
	}
	if (a.Kind == "" || a.Kind == "Network device" || a.Confidence == "Unknown") && b.Kind != "" && b.Kind != "Network device" {
		a.Kind = b.Kind
		a.Confidence = b.Confidence
		a.Identification = b.Identification
	}
	a.Addresses = orderAddresses(distinct(a.Addresses, b.Addresses))
	a.MACs = distinct(a.MACs, b.MACs)
	a.Interfaces = distinct(a.Interfaces, b.Interfaces)
	a.Sources = distinct(a.Sources, b.Sources)
	a.Aliases = distinct(a.Aliases, b.Aliases)
	a.Services = distinct(a.Services, b.Services)
	if len(a.Addresses) > 0 {
		a.IP = a.Addresses[0]
	}
	if a.MAC == "" && len(a.MACs) > 0 {
		a.MAC = a.MACs[0]
	}
	for _, pair := range []struct {
		dst *string
		src string
	}{{&a.Pub, b.Pub}, {&a.UUID, b.UUID}, {&a.Name, b.Name}, {&a.Hostname, b.Hostname}, {&a.Platform, b.Platform}, {&a.BroadcastName, b.BroadcastName}, {&a.Address, b.Address}, {&a.Manufacturer, b.Manufacturer}, {&a.Vendor, b.Vendor}, {&a.Model, b.Model}, {&a.Kind, b.Kind}, {&a.Identification, b.Identification}, {&a.Confidence, b.Confidence}} {
		if *pair.dst == "" {
			*pair.dst = pair.src
		}
	}
	if b.Seen > a.Seen {
		a.Seen = b.Seen
	}
	statusRank := func(status string) int {
		switch status {
		case "SpearHead online":
			return 4
		case "Responding":
			return 3
		case "Reachable":
			return 2
		case "Known neighbor":
			return 1
		}
		return 0
	}
	if a.Status == "" || statusRank(b.Status) > statusRank(a.Status) {
		a.Status = b.Status
	}
	if b.Pub != "" {
		a.Status = "SpearHead online"
	}
	return a
}

// Inventory merges observations into device records without equating identities
// by a guessed name or manufacturer. The local machine is always exactly one row.
func Inventory(s Snapshot, observations []Device, selfPub, broadcastName string) []Device {
	self := Device{ID: "self", Self: true, Name: s.Hostname, Hostname: s.Hostname, Platform: s.Platform, BroadcastName: broadcastName, Status: "This device", Kind: "Computer", Confidence: "Known", Identification: "Local computer", Source: "Local interfaces", Addresses: []string{}, MACs: []string{}, Interfaces: []string{}, Services: []string{}}
	localIPs, localMACs := map[string]bool{}, map[string]bool{}
	primary := ""
	primaryMAC := ""
	for _, i := range s.Interfaces {
		if !i.Up {
			continue
		}
		if mac := validMAC(i.MAC); mac != "" {
			localMACs[mac] = true
			self.MACs = append(self.MACs, mac)
		}
		self.Interfaces = append(self.Interfaces, i.Name)
		for _, a := range i.Addresses {
			ip, _, err := net.ParseCIDR(a)
			if err != nil || ip.IsLoopback() {
				continue
			}
			value := ip.String()
			localIPs[value] = true
			self.Addresses = append(self.Addresses, value)
			if ip.To4() != nil && !i.Virtual && (primary == "" || i.DefaultRoute) {
				primary = value
				primaryMAC = validMAC(i.MAC)
			}
		}
	}
	self = canonical(self)
	if primary != "" {
		self.IP = primary
		if primaryMAC != "" {
			self.MAC = primaryMAC
		}
	}
	self.Interface = strings.Join(self.Interfaces, ", ")
	self.Vendor = vendorsFor([]Device{self})[self.MAC]
	raw := append(append([]Device{}, s.Devices...), observations...)
	sort.SliceStable(raw, func(i, j int) bool {
		if (raw[i].Pub != "") != (raw[j].Pub != "") {
			return raw[i].Pub != ""
		}
		if (raw[i].UUID != "") != (raw[j].UUID != "") {
			return raw[i].UUID != ""
		}
		rank := func(d Device) int {
			if d.UUID != "" {
				return 4
			}
			if strings.Contains(d.Source, "mDNS") {
				return 3
			}
			if d.Hostname != "" {
				return 2
			}
			if d.Name != "" && d.Name != d.IP && d.Name != d.Kind {
				return 1
			}
			return 0
		}
		if rank(raw[i]) != rank(raw[j]) {
			return rank(raw[i]) > rank(raw[j])
		}
		return raw[i].IP < raw[j].IP
	})
	groups := []Device{}
	gateway := strings.Fields(s.Gateway)
	gatewayIP := ""
	if len(gateway) > 0 {
		gatewayIP = cleanIP(gateway[0])
	}
	gatewayMACs := []string{}
	for _, candidate := range raw {
		d := canonical(candidate)
		if overlaps(d.Addresses, []string{gatewayIP}) {
			gatewayMACs = distinct(gatewayMACs, d.MACs)
		}
	}
	for _, candidate := range raw {
		d := canonical(candidate)
		if len(d.Addresses) == 0 {
			continue
		}
		own := d.Self || d.Pub != "" && d.Pub == selfPub
		for _, ip := range d.Addresses {
			own = own || localIPs[ip]
		}
		for _, mac := range d.MACs {
			own = own || localMACs[mac]
		}
		if own {
			continue
		}
		match := -1
		for i, g := range groups {
			if sameDevice(g, d, gatewayIP, gatewayMACs) {
				match = i
				break
			}
		}
		if match >= 0 {
			groups[match] = mergeDevice(groups[match], d)
		} else {
			groups = append(groups, d)
		}
	}
	for i := range groups {
		d := &groups[i]
		switch {
		case d.Pub != "":
			d.ID = d.Pub
		case d.UUID != "":
			d.ID = "upnp/" + d.UUID
		case len(d.MACs) > 0:
			d.ID = "mac/" + d.MACs[0]
		default:
			d.ID = "ip/" + d.IP
		}
		if (d.Status == "Responding" || d.Status == "Reachable") && d.Seen > 0 && time.Now().Unix()-d.Seen > 30 {
			d.Status = "Seen recently"
		}
		d.Interface = strings.Join(d.Interfaces, ", ")
		d.Source = strings.Join(d.Sources, ", ")
		if gatewayIP != "" && overlaps(d.Addresses, []string{gatewayIP}) {
			d.Kind = "Router"
			d.Confidence = "Known"
			d.Identification = "Default gateway"
		}
		if d.Kind == "" {
			d.Kind = "Network device"
		}
		if d.Confidence == "" {
			d.Confidence = "Unknown"
		}
		if d.Name == "" {
			if d.Hostname != "" {
				d.Name = d.Hostname
			} else if d.Model != "" {
				d.Name = strings.TrimSpace(d.Manufacturer + " " + d.Model)
			} else if d.Kind != "Network device" {
				d.Name = d.Kind
			} else if d.Vendor != "" {
				d.Name = d.Vendor + " device"
			} else {
				d.Name = d.IP
			}
		}
	}
	// Proxy ARP and independently scoped link-local observations can share the
	// same candidate identity. Keep their row IDs distinct and deterministic.
	idCounts := map[string]int{}
	for _, d := range groups {
		idCounts[d.ID]++
	}
	for i := range groups {
		d := &groups[i]
		if idCounts[d.ID] > 1 {
			d.ID += "/" + d.IP + "/" + strings.Join(d.Interfaces, ",")
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if (a.Kind == "Router") != (b.Kind == "Router") {
			return a.Kind == "Router"
		}
		if (a.Pub != "") != (b.Pub != "") {
			return a.Pub != ""
		}
		if a.Name != b.Name {
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		}
		return a.IP < b.IP
	})
	return append([]Device{self}, groups...)
}
