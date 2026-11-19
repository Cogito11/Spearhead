package identity

import (
	"os"
	"runtime"
)

// LocalDevice is descriptive information, never a replacement for key verification.
func LocalDevice() (name, platform string) {
	name, _ = os.Hostname()
	return CleanName(name), runtime.GOOS + "/" + runtime.GOARCH
}
