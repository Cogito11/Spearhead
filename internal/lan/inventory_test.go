package lan

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"
)

func TestInventoryCombinesAllLocalAddressesAndVirtualInterfaces(t *testing.T) {
	s := Snapshot{Hostname: "My PC", Platform: "linux/amd64", Interfaces: []Interface{{Name: "eth0", Up: true, DefaultRoute: true, MAC: "00:11:22:33:44:55", Addresses: []string{"192.168.1.2/24", "fe80::2/64", "2001:db8::2/64"}}, {Name: "wlan0", Up: true, MAC: "00:11:22:33:44:56", Addresses: []string{"192.168.1.3/24", "2001:db8::3/64"}}, {Name: "docker0", Up: true, Virtual: true, MAC: "02:42:aa:bb:cc:dd", Addresses: []string{"172.17.0.1/16"}}}, Devices: []Device{{IP: "192.168.1.2", MAC: "00:11:22:33:44:55"}}}
	original, _ := json.Marshal(s)
	got := Inventory(s, []Device{{Pub: "me", IP: "192.168.1.3", Name: "Me"}, {Pub: "another-local-profile", IP: "192.168.1.2"}}, "me", "Broadcast")
	if len(got) != 1 || !got[0].Self || got[0].ID != "self" || len(got[0].Addresses) != 6 || got[0].IP != "192.168.1.2" || got[0].MAC != "00:11:22:33:44:55" {
		t.Fatalf("local duplicates: %+v", got)
	}
	got[0].Addresses[0] = "mutated"
	after, _ := json.Marshal(s)
	if string(original) != string(after) {
		t.Fatal("inventory mutated snapshot")
	}
}
func TestInventoryMergesIPv4IPv6PeersAndServiceMetadata(t *testing.T) {
	s := Snapshot{Hostname: "Here", Devices: []Device{{IP: "192.168.1.20", MAC: "00:11:22:33:44:55", Interface: "eth0", Source: "Neighbor table"}, {IP: "fe80::20", MAC: "00:11:22:33:44:55", Interface: "eth0", Source: "Neighbor table"}}}
	extra := []Device{{Pub: "alice", IP: "192.168.1.20", Name: "Alice laptop", BroadcastName: "Alice", Source: "Signed announcement", Platform: "linux/amd64"}, {IP: "192.168.1.20", Name: "Alice PC", Hostname: "alice.local", Services: []string{"_workstation._tcp"}, Source: "mDNS / DNS-SD"}}
	got := Inventory(s, extra, "self", "Me")
	if len(got) != 2 || got[1].Pub != "alice" || len(got[1].Addresses) != 2 || got[1].Name != "Alice laptop" || got[1].Hostname != "alice.local" || len(got[1].Services) != 1 {
		t.Fatalf("not merged: %+v", got)
	}
	again := Inventory(s, extra, "self", "Me")
	if got[1].ID != again[1].ID {
		t.Fatal("unstable identity")
	}
}
func TestInventoryDoesNotMergeByHostnameOrVendorOrConflictingIdentity(t *testing.T) {
	s := Snapshot{Devices: []Device{{IP: "192.168.1.10", MAC: "00:11:22:33:44:10", Name: "Printer", Vendor: "Vendor"}, {IP: "192.168.1.11", MAC: "00:11:22:33:44:11", Name: "Printer", Vendor: "Vendor"}}}
	peers := []Device{{IP: "192.168.1.10", MAC: "00:11:22:33:44:10", Pub: "a"}, {IP: "192.168.1.10", MAC: "00:11:22:33:44:10", Pub: "b"}}
	got := Inventory(s, peers, "self", "")
	if len(got) != 4 {
		t.Fatalf("distinct identities collapsed: %+v", got)
	}
}
func TestInventoryDoesNotCollapseRouterProxyARPHosts(t *testing.T) {
	s := Snapshot{Gateway: "192.168.1.1 (eth0)", Devices: []Device{{IP: "192.168.1.1", MAC: "00:11:22:33:44:55"}, {IP: "192.168.1.50", MAC: "00:11:22:33:44:55"}, {IP: "fe80::1", MAC: "00:11:22:33:44:55"}, {IP: "192.168.1.51", MAC: "00:11:22:33:44:55"}}}
	got := Inventory(s, nil, "self", "")
	if len(got) != 4 {
		t.Fatalf("proxy ARP collapsed hosts: %+v", got)
	}
	ids := map[string]bool{}
	for _, d := range got {
		if ids[d.ID] {
			t.Fatalf("proxy ARP produced duplicate row ID: %s", d.ID)
		}
		ids[d.ID] = true
	}
	if got[1].Kind != "Router" {
		t.Fatalf("gateway identification missing: %+v", got)
	}
}
func TestScanOmitsOtherOwnAddressesAndVirtualNetworks(t *testing.T) {
	ifs := []Interface{{Up: true, Addresses: []string{"192.168.1.2/24", "192.168.1.3/24"}}, {Up: true, Virtual: true, Addresses: []string{"172.17.0.1/16"}}}
	for _, ip := range scanTargets(ifs) {
		if ip == "192.168.1.2" || ip == "192.168.1.3" || strings.HasPrefix(ip, "172.17.") {
			t.Fatalf("scanned self/virtual address %s", ip)
		}
	}
}
func TestAvahiIdentifiesPrinterAndDecodesNames(t *testing.T) {
	ifs := []Interface{{Name: "eth0", Up: true, Addresses: []string{"192.168.1.2/24"}}}
	raw := []byte("=;eth0;IPv4;Office\\032Printer;_ipp._tcp;local;printer.local;192.168.1.8;631;\"ty=LaserJet\\032Pro\" \"manufacturer=HP\"\n=;eth0;IPv4;Self;_ssh._tcp;local;my.local;192.168.1.2;22;\n=;eth0;IPv4;Foreign;_ipp._tcp;local;other.local;8.8.8.8;631;\n")
	got := parseAvahi(raw, ifs)
	if len(got) != 1 || got[0].Name != "Office Printer" || got[0].Kind != "Printer" || got[0].Model != "LaserJet Pro" || got[0].Manufacturer != "HP" || got[0].Confidence != "Advertised" {
		t.Fatalf("mDNS result %+v", got)
	}
}
func TestUPnPDescriptionAndSSRFBoundary(t *testing.T) {
	good := `<root xmlns="urn:schemas-upnp-org:device-1-0"><device><deviceType>urn:schemas-upnp-org:device:MediaRenderer:1</deviceType><friendlyName>Living room TV</friendlyName><manufacturer>Samsung</manufacturer><modelName>Smart TV</modelName><UDN>uuid:tv-123</UDN></device></root>`
	got, err := parseDescription(strings.NewReader(good), "192.168.1.8")
	if err != nil || got.Kind != "Media device" || got.Name != "Living room TV" || got.Model != "Smart TV" || got.UUID != "uuid:tv-123" {
		t.Fatalf("UPnP: %+v %v", got, err)
	}
	sender := net.ParseIP("192.168.1.8")
	if !descriptionURL("http://192.168.1.8:1234/description.xml", sender) {
		t.Fatal("valid local URL rejected")
	}
	for _, url := range []string{"http://127.0.0.1:7780/", "http://169.254.169.254/metadata", "http://example.com/", "file:///etc/passwd", "http://user:password@192.168.1.8/", "http://192.168.1.8:99999/", "http://192.168.1.9/"} {
		if descriptionURL(url, sender) {
			t.Fatalf("foreign/private endpoint allowed %s", url)
		}
	}
}
func TestVendorLookupDoesNotAttributePrivateMACs(t *testing.T) {
	devices := []Device{{MAC: "dc:a6:32:00:00:01"}, {MAC: "de:a6:32:00:00:01"}}
	got := vendorsFor(devices)
	if !strings.Contains(strings.ToLower(got[devices[0].MAC]), "raspberry") {
		t.Fatalf("offline vendor %+v", got)
	}
	if got[devices[1].MAC] != "" {
		t.Fatal("locally assigned MAC was falsely attributed")
	}
}
func TestGuessesAreLabeledAndDoNotPretendSSHMeansAnOS(t *testing.T) {
	d := Device{Services: []string{"22/tcp · SSH"}}
	applyGuess(&d)
	if d.Kind != "" || d.Platform != "" {
		t.Fatalf("invented device OS: %+v", d)
	}
	d.Services = append(d.Services, "631/tcp · Printing")
	applyGuess(&d)
	if d.Kind != "Printer" || d.Confidence != "Likely" || d.Identification == "" {
		t.Fatalf("unlabeled guess: %+v", d)
	}
}

func TestHintCacheDoesNotKeepObsoleteNeighborsAsGhosts(t *testing.T) {
	snapshot := Snapshot{Devices: []Device{{IP: "192.168.1.20", MAC: "00:11:22:33:44:20"}}}
	monitor := &Monitor{hints: []Device{{IP: "192.168.1.20", Hostname: "current.local"}, {IP: "192.168.1.21", Source: "Neighbor table", Seen: time.Now().Unix()}, {IP: "192.168.1.22", Source: "mDNS / DNS-SD", Seen: time.Now().Unix() - 500}, {IP: "192.168.1.23", Source: "mDNS / DNS-SD", Seen: time.Now().Unix()}}}
	got := monitor.withHints(snapshot)
	if len(got.Devices) != 3 {
		t.Fatalf("stale probe/cache devices retained: %+v", got.Devices)
	}
	if len(snapshot.Devices) != 1 {
		t.Fatal("hint application mutated cached observations")
	}
}
func TestSourceMetadataDoesNotGrowOnRepeatedRefreshes(t *testing.T) {
	record := Device{IP: "192.168.1.8", MAC: "00:11:22:33:44:55", Source: "Neighbor table, Reverse DNS", Sources: []string{"Neighbor table", "Reverse DNS"}}
	for i := 0; i < 20; i++ {
		record = canonical(record)
		record.Source = strings.Join(record.Sources, ", ")
	}
	if len(record.Sources) != 2 {
		t.Fatalf("source metadata duplicated: %+v", record.Sources)
	}
}
