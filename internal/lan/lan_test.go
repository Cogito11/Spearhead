package lan

import (
	"net"
	"strings"
	"testing"
)

func TestScanStaysOnPrivateLocalSegments(t *testing.T) {
	ifs := []Interface{{Up: true, Addresses: []string{"192.168.12.9/16", "10.2.0.1/30", "8.8.8.1/24", "2001:db8::1/64"}}, {Up: false, Addresses: []string{"172.16.0.1/24"}}}
	targets := scanTargets(ifs)
	if len(targets) != 254 {
		t.Fatalf("unexpected target count %d", len(targets))
	}
	for _, target := range targets {
		if target == "192.168.12.9" {
			t.Fatal("scanned self")
		}
		if !strings.HasPrefix(target, "192.168.12.") && target != "10.2.0.2" {
			t.Fatalf("scanned outside local segment: %s", target)
		}
	}
}
func TestScanTargetBudget(t *testing.T) {
	ifs := []Interface{}
	for i := 0; i < 10; i++ {
		ifs = append(ifs, Interface{Up: true, Addresses: []string{net.IPv4(10, byte(i), 0, 1).String() + "/16"}})
	}
	if n := len(scanTargets(ifs)); n > 1024 || n == 0 {
		t.Fatalf("scan budget %d", n)
	}
}
func TestNeighborInventoryOmitsUnreachableAndMalformed(t *testing.T) {
	raw := []byte(`[{"dst":"192.168.1.8","dev":"eth0","lladdr":"aa:bb:cc:dd:ee:ff","state":["REACHABLE"]},{"dst":"192.168.1.9","dev":"eth0","state":["FAILED"]},{"dst":"192.168.1.10","dev":"eth0","lladdr":"00:11:22:33:44:55","state":["INCOMPLETE"]},{"dst":"garbage","dev":"eth0","lladdr":"aa","state":["REACHABLE"]}]`)
	got := parseNeighbors(raw, 123)
	if len(got) != 1 || got[0].IP != "192.168.1.8" || got[0].Seen != 123 || got[0].Interface != "eth0" {
		t.Fatalf("inventory: %+v", got)
	}
}
