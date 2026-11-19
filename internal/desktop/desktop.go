// Package desktop launches the native Qt Quick tray host and its disposable
// QML window. The Go service remains independent of the graphical runtime.
package desktop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func Helper() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	path := filepath.Join(filepath.Dir(exe), "spearhead-desktop")
	if st, err := os.Stat(path); err == nil && !st.IsDir() && st.Mode()&0111 != 0 {
		return path
	}
	return ""
}

func Start(ctx context.Context, url string, background bool) (*exec.Cmd, error) {
	args := []string{"--url-stdin"}
	if background {
		args = append(args, "--background")
	}
	c := exec.CommandContext(ctx, Helper(), args...)
	c.Stdin = strings.NewReader(url + "\n")
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c, c.Start()
}
