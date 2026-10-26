package secure

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultIdleTimeout is how long the store's key lasts without being used,
// unless the computer's settings say otherwise. It matches sudo's default.
const DefaultIdleTimeout = 15 * time.Minute

// settingsFile is the per-computer security settings, e.g.
//
//	{"key_timeout": "5m"}
//
// "0" keeps the key until the app closes.
type settingsFile struct {
	KeyTimeout string `json:"key_timeout"`
}

// Paths for tests.
var (
	systemSettingsPath = "/etc/mint-interfaces/security.json"
	userSettingsPath   = func() string {
		dir, err := os.UserConfigDir()
		if err != nil {
			return ""
		}
		return filepath.Join(dir, "mint-interfaces", "security.json")
	}
)

// IdleTimeout is this computer's key timeout. In order of precedence:
// $MINT_KEY_TIMEOUT (e.g. "5m", "0"), the user's security.json in
// ~/.config/mint-interfaces, the administrator's /etc/mint-interfaces/
// security.json, then DefaultIdleTimeout. The source is returned too, so
// apps can say where the value came from.
func IdleTimeout() (d time.Duration, source string) {
	if v := strings.TrimSpace(os.Getenv("MINT_KEY_TIMEOUT")); v != "" {
		if d, ok := parseTimeout(v); ok {
			return d, "MINT_KEY_TIMEOUT"
		}
	}
	for _, path := range []string{userSettingsPath(), systemSettingsPath} {
		if path == "" {
			continue
		}
		b, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var s settingsFile
		if json.Unmarshal(b, &s) == nil && s.KeyTimeout != "" {
			if d, ok := parseTimeout(s.KeyTimeout); ok {
				return d, path
			}
		}
	}
	return DefaultIdleTimeout, "default"
}

func parseTimeout(v string) (time.Duration, bool) {
	if v == "0" {
		return 0, true
	}
	d, err := time.ParseDuration(v)
	if err != nil || d < 0 {
		return 0, false
	}
	return d, true
}
