package discovery

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"spearhead/internal/identity"
)

func TestSignedBeaconValidation(t *testing.T) {
	me, err := identity.Create(t.TempDir(), "Alex")
	if err != nil {
		t.Fatal(err)
	}
	b := signedBeacon(me, 7777)
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := validBeacon(raw, "another-public-key")
	if !ok || got.Pub != b.Pub || got.Name != "Alex" || got.Port != 7777 {
		t.Fatalf("valid beacon rejected: %#v, %v", got, ok)
	}
	b.Name = "Mallory"
	raw, _ = json.Marshal(b)
	if _, ok := validBeacon(raw, "another-public-key"); ok {
		t.Fatal("altered beacon accepted")
	}
}

func TestBeaconRejectsLegacyAndSelf(t *testing.T) {
	me, err := identity.Create(t.TempDir(), "Alex")
	if err != nil {
		t.Fatal(err)
	}
	b := signedBeacon(me, 7777)
	raw, _ := json.Marshal(b)
	if _, ok := validBeacon(raw, b.Pub); ok {
		t.Fatal("self beacon accepted")
	}
	b.V = 1
	raw, _ = json.Marshal(b)
	if _, ok := validBeacon(raw, "another-public-key"); ok {
		t.Fatal("legacy unsigned beacon accepted")
	}
}

// Two instances must share a discovery port and both receive multicast and
// broadcast. This catches the exclusive broadcast bind that broke multicast.
func TestSharedPortReceivesLANBeacons(t *testing.T) {
	reserve, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	port := reserve.LocalAddr().(*net.UDPAddr).Port
	reserve.Close()
	errors := []error{}
	report := func(err error) { errors = append(errors, err) }
	first, second := listen(port, report), listen(port, report)
	for _, cs := range [][]*net.UDPConn{first, second} {
		for _, c := range cs {
			t.Cleanup(func() { c.Close() })
		}
	}
	if len(errors) > 0 || len(first) == 0 || len(second) == 0 {
		t.Fatalf("discovery port cannot be shared: %v", errors)
	}
	ifs, _ := net.Interfaces()
	hasLAN := false
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp != 0 && ifc.Flags&net.FlagMulticast != 0 && ifc.Flags&net.FlagLoopback == 0 {
			hasLAN = true
		}
	}
	if !hasLAN {
		t.Skip("no multicast LAN interface")
	}
	me, err := identity.Create(t.TempDir(), "Alex")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(signedBeacon(me, 7777))
	for _, target := range []*net.UDPAddr{{IP: multicastAddr, Port: port}, {IP: net.IPv4bcast, Port: port}} {
		t.Run(target.IP.String(), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			// Fresh listeners per subtest because receive closes its socket on cancel.
			cs1, cs2 := listen(port, func(err error) { t.Error(err) }), listen(port, func(err error) { t.Error(err) })
			ch1, ch2 := make(chan string, 32), make(chan string, 32)
			for _, c := range cs1 {
				go receive(ctx, c, "other", func(_, _, addr string) { ch1 <- addr })
			}
			for _, c := range cs2 {
				go receive(ctx, c, "other", func(_, _, addr string) { ch2 <- addr })
			}
			sender, err := net.DialUDP("udp4", nil, target)
			if err != nil {
				t.Fatal(err)
			}
			defer sender.Close()
			if _, err := sender.Write(raw); err != nil {
				t.Fatal(err)
			}
			for _, ch := range []chan string{ch1, ch2} {
				select {
				case addr := <-ch:
					if !strings.HasSuffix(addr, ":7777") {
						t.Fatalf("incorrect return address %q", addr)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("LAN beacon not received by both peers")
				}
			}
		})
	}
}

func TestDeviceInformationIsSignedAndLegacyCompatible(t *testing.T) {
	me, err := identity.Create(t.TempDir(), "Alex")
	if err != nil {
		t.Fatal(err)
	}
	b := signedBeacon(me, 7777)
	raw, _ := json.Marshal(b)
	if got, ok := validBeacon(raw, "other"); !ok || got.Device != b.Device || got.Platform != b.Platform {
		t.Fatal("signed device data rejected")
	}
	b.Device = "forged-device"
	raw, _ = json.Marshal(b)
	if _, ok := validBeacon(raw, "other"); ok {
		t.Fatal("forged device data accepted")
	}
	b.Device, b.Platform, b.InfoSig = "", "", ""
	raw, _ = json.Marshal(b)
	if _, ok := validBeacon(raw, "other"); !ok {
		t.Fatal("legacy v2 announcement rejected")
	}
}
