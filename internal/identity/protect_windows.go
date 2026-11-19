//go:build windows

package identity

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"
)

// Windows Data Protection API encrypts the identity seed for the current
// Windows user. Copying identity.json to another account or machine no longer
// exposes the signing key; it will instead fail to unlock there.
const cryptProtectUIForbidden = 0x1

type dataBlob struct {
	Len  uint32
	Data *byte
}

var (
	crypt32            = syscall.NewLazyDLL("crypt32.dll")
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	cryptProtectData   = crypt32.NewProc("CryptProtectData")
	cryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	localFree          = kernel32.NewProc("LocalFree")
)

func keyProtectionAvailable() bool { return true }

func protectSeed(seed []byte) ([]byte, error)     { return crypt(seed, true) }
func unprotectSeed(sealed []byte) ([]byte, error) { return crypt(sealed, false) }

func crypt(in []byte, protect bool) ([]byte, error) {
	if len(in) == 0 {
		return nil, errors.New("empty protected data")
	}
	src := dataBlob{Len: uint32(len(in)), Data: &in[0]}
	var dst dataBlob
	var ok uintptr
	var callErr error
	if protect {
		ok, _, callErr = cryptProtectData.Call(
			uintptr(unsafe.Pointer(&src)), 0, 0, 0, 0, cryptProtectUIForbidden, uintptr(unsafe.Pointer(&dst)))
	} else {
		ok, _, callErr = cryptUnprotectData.Call(
			uintptr(unsafe.Pointer(&src)), 0, 0, 0, 0, cryptProtectUIForbidden, uintptr(unsafe.Pointer(&dst)))
	}
	if ok == 0 {
		if callErr != syscall.Errno(0) {
			return nil, fmt.Errorf("Windows DPAPI: %w", callErr)
		}
		return nil, errors.New("Windows DPAPI failed")
	}
	defer localFree.Call(uintptr(unsafe.Pointer(dst.Data)))
	return append([]byte(nil), unsafe.Slice(dst.Data, int(dst.Len))...), nil
}
