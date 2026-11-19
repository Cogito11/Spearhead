// Package config loads SpearHead's settings and resolves its data directory.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Config struct {
	ListenPort           int  `json:"listen_port"`
	DiscoveryPort        int  `json:"discovery_port"`
	UIPort               int  `json:"ui_port"`
	Discovery            bool `json:"discovery"`
	DefaultRetentionDays int  `json:"default_retention_days"`
	SecretDetection      bool `json:"secret_detection"`
}

func Default() *Config {
	return &Config{ListenPort: 7777, DiscoveryPort: 7778, UIPort: 7780, Discovery: true, DefaultRetentionDays: 90, SecretDetection: true}
}

// Home resolves the data directory: explicit flag, then SPEARHEAD_HOME, then the
// per-user config directory.
func Home(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	if v := os.Getenv("SPEARHEAD_HOME"); v != "" {
		return v
	}
	// Honor the previous name when upgrading an existing installation.
	if v := os.Getenv("RACKET_HOME"); v != "" {
		return v
	}
	d, err := os.UserConfigDir()
	if err != nil {
		d = "."
	}
	home := filepath.Join(d, "spearhead")
	legacy := filepath.Join(d, "racket")
	if _, err := os.Stat(filepath.Join(home, "identity.json")); os.IsNotExist(err) {
		if _, err := os.Stat(filepath.Join(legacy, "identity.json")); err == nil {
			return legacy
		}
	}
	return home
}

// Load reads config.json, falling back to defaults for missing keys or file.
func Load(home string) (*Config, error) {
	c := Default()
	b, err := os.ReadFile(filepath.Join(home, "config.json"))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	return c, json.Unmarshal(b, c)
}

// Save writes config.json.
func Save(home string, c *Config) error {
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(filepath.Join(home, "config.json"), b, 0o600)
}
