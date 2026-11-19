//go:build !windows

package identity

import "errors"

// Other platforms currently retain the existing file-permission protection.
// This explicit stub keeps their on-disk format portable while Windows uses
// DPAPI, which is tied to the signed-in account on that computer.
func keyProtectionAvailable() bool { return false }

func protectSeed([]byte) ([]byte, error) {
	return nil, errors.New("platform key protection is unavailable")
}
func unprotectSeed([]byte) ([]byte, error) {
	return nil, errors.New("platform key protection is unavailable")
}
