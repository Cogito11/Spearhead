package node

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"strconv"
	"strings"

	"spearhead/internal/identity"
)

const invitePrefix = "rkt1:"

type invite struct {
	K string   `json:"k"`
	N string   `json:"n"`
	A []string `json:"a,omitempty"`
}

// LocalAddrs lists this machine's non-loopback IPv4 addresses with port.
func LocalAddrs(port int) []string {
	var out []string
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil && !ipn.IP.IsLoopback() && !ipn.IP.IsLinkLocalUnicast() {
			out = append(out, net.JoinHostPort(ipn.IP.String(), strconv.Itoa(port)))
		}
	}
	return out
}

// InviteCode encodes this node's public key, name, and address hints.
func (n *Node) InviteCode() string {
	b, _ := json.Marshal(invite{K: identity.Encode(n.ID.Pub), N: n.ID.DisplayName(), A: LocalAddrs(n.advertPort())})
	return invitePrefix + base64.RawURLEncoding.EncodeToString(b)
}

// IsInvite reports whether s looks like an invite code.
func IsInvite(s string) bool { return strings.HasPrefix(s, invitePrefix) }

// ParseInvite decodes an invite code.
func ParseInvite(code string) (pub ed25519.PublicKey, name string, addrs []string, err error) {
	raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(strings.TrimSpace(code), invitePrefix))
	if err != nil {
		return nil, "", nil, errors.New("invalid invite code")
	}
	var iv invite
	if err := json.Unmarshal(raw, &iv); err != nil {
		return nil, "", nil, errors.New("invalid invite code")
	}
	pub, err = identity.Decode(iv.K)
	if err != nil {
		return nil, "", nil, err
	}
	return pub, identity.CleanName(iv.N), iv.A, nil
}

// LocalAddresses reports the actual bound port when running.
func (n *Node) LocalAddresses() []string { return LocalAddrs(n.advertPort()) }
