// Package discovery announces and listens for SpearHead peers on a local network.
// Beacons are signed identity hints; the encrypted handshake remains the final
// proof before a peer can send or receive a message.
package discovery

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"time"

	"spearhead/internal/identity"
)

const (
	interval      = 5 * time.Second
	beaconVersion = 2
)

var multicastAddr = net.IPv4(239, 255, 77, 77)

type Beacon struct {
	V        int    `json:"v"`
	Pub      string `json:"pub"`
	Name     string `json:"name"`
	Port     int    `json:"port"`
	Sig      string `json:"sig"`
	Device   string `json:"device,omitempty"`
	Platform string `json:"platform,omitempty"`
	InfoSig  string `json:"info_sig,omitempty"`
}

func (b Beacon) signingInput() []byte {
	return []byte(fmt.Sprintf("racket/v2/discovery\n%s\n%s\n%d", b.Pub, b.Name, b.Port))
}

func (b Beacon) infoInput() []byte {
	return []byte(fmt.Sprintf("spearhead/v1/device\n%s\n%s\n%s\n%s", b.Pub, b.Sig, b.Device, b.Platform))
}

func signedBeacon(me *identity.Identity, port int) Beacon {
	b := Beacon{V: beaconVersion, Pub: identity.Encode(me.Pub), Name: identity.CleanName(me.DisplayName()), Port: port}
	b.Sig = base64.RawURLEncoding.EncodeToString(ed25519.Sign(me.Priv, b.signingInput()))
	b.Device, b.Platform = identity.LocalDevice()
	b.InfoSig = base64.RawURLEncoding.EncodeToString(ed25519.Sign(me.Priv, b.infoInput()))
	return b
}

// validBeacon rejects malformed, altered, legacy, and unsigned announcements.
func validBeacon(raw []byte, self string) (Beacon, bool) {
	var b Beacon
	if len(raw) > 1024 || json.Unmarshal(raw, &b) != nil || b.V != beaconVersion || b.Pub == self || b.Port < 1 || b.Port > 65535 {
		return Beacon{}, false
	}
	pub, err := identity.Decode(b.Pub)
	if err != nil || identity.Encode(pub) != b.Pub || identity.CleanName(b.Name) != b.Name {
		return Beacon{}, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(b.Sig)
	if err != nil || len(sig) != ed25519.SignatureSize || !ed25519.Verify(pub, b.signingInput(), sig) {
		return Beacon{}, false
	}
	if b.Device != "" || b.Platform != "" || b.InfoSig != "" {
		info, err := base64.RawURLEncoding.DecodeString(b.InfoSig)
		if err != nil || identity.CleanName(b.Device) != b.Device || identity.CleanName(b.Platform) != b.Platform || !ed25519.Verify(pub, b.infoInput(), info) {
			return Beacon{}, false
		}
	}
	return b, true
}

func broadcastAddrs(port int) []*net.UDPAddr {
	seen := map[string]bool{}
	add := func(ip net.IP) { seen[ip.String()] = true }
	add(net.IPv4bcast)
	ifs, _ := net.Interfaces()
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagBroadcast == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			ip, mask := ipn.IP.To4(), ipn.Mask
			if len(mask) == 16 {
				mask = mask[12:]
			}
			if len(mask) != net.IPv4len {
				continue
			}
			b := make(net.IP, net.IPv4len)
			for i := range b {
				b[i] = ip[i] | ^mask[i]
			}
			add(b)
		}
	}
	keys := make([]string, 0, len(seen))
	for ip := range seen {
		keys = append(keys, ip)
	}
	sort.Strings(keys)
	out := make([]*net.UDPAddr, 0, len(keys))
	for _, ip := range keys {
		out = append(out, &net.UDPAddr{IP: net.ParseIP(ip), Port: port})
	}
	return out
}

func receive(ctx context.Context, conn *net.UDPConn, self string, onPeer func(pub, name, addr string)) {
	receiveInfo(ctx, conn, self, func(b Beacon, addr string) { onPeer(b.Pub, b.Name, addr) })
}

func receiveInfo(ctx context.Context, conn *net.UDPConn, self string, onPeer func(Beacon, string)) {
	defer conn.Close()
	go func() { <-ctx.Done(); conn.Close() }()
	buf := make([]byte, 1024)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		b, ok := validBeacon(buf[:n], self)
		if !ok || from.IP.IsUnspecified() || from.IP.IsMulticast() {
			continue
		}
		onPeer(b, net.JoinHostPort(from.IP.String(), itoa(b.Port)))
	}
}

// listen opens reusable multicast sockets before falling back to a plain UDP
// socket. Multicast sockets bind the wildcard address and also receive LAN
// broadcasts; binding a separate non-reusable broadcast socket first prevents
// the multicast socket from binding on Linux.
func listen(port int, errf func(error)) []*net.UDPConn {
	var conns []*net.UDPConn
	ifs, err := net.Interfaces()
	if err != nil {
		errf(err)
	}
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagMulticast == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		hasIPv4 := false
		for _, addr := range addrs {
			if ipn, ok := addr.(*net.IPNet); ok && ipn.IP.To4() != nil {
				hasIPv4 = true
				break
			}
		}
		if !hasIPv4 {
			continue
		}
		conn, err := net.ListenMulticastUDP("udp4", &ifc, &net.UDPAddr{IP: multicastAddr, Port: port})
		if err != nil {
			errf(fmt.Errorf("multicast on %s: %w", ifc.Name, err))
			continue
		}
		conns = append(conns, conn)
	}
	if len(conns) == 0 {
		conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: port})
		if err != nil {
			errf(fmt.Errorf("broadcast listener: %w", err))
		} else {
			conns = append(conns, conn)
		}
	}
	return conns
}

func announce(msg []byte, port int, conns []*net.UDPConn) {
	for _, a := range broadcastAddrs(port) {
		if c, err := net.DialUDP("udp4", nil, a); err == nil {
			_, _ = c.Write(msg)
			_ = c.Close()
		}
	}
	// Each multicast socket has its outgoing interface set by Go. Announce on
	// every LAN interface, including when the default route is a VPN.
	for _, c := range conns {
		_, _ = c.WriteToUDP(msg, &net.UDPAddr{IP: multicastAddr, Port: port})
	}
}

// Run announces and listens until ctx is cancelled, using both broadcast and
// multicast on all active IPv4 LAN interfaces.
func Run(ctx context.Context, port, tcpPort int, me *identity.Identity, onPeer func(pub, name, addr string), errf func(error)) {
	RunWithInfo(ctx, port, tcpPort, me, func(b Beacon, addr string) { onPeer(b.Pub, b.Name, addr) }, errf)
}

func RunWithInfo(ctx context.Context, port, tcpPort int, me *identity.Identity, onPeer func(Beacon, string), errf func(error)) {
	self := identity.Encode(me.Pub)
	conns := listen(port, errf)
	if len(conns) == 0 {
		return
	}
	for _, conn := range conns {
		go receiveInfo(ctx, conn, self, onPeer)
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		msg, _ := json.Marshal(signedBeacon(me, tcpPort))
		announce(msg, port, conns)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
