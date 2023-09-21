package conf

import "strings"

// SanitizeAPIKeyConfig strips newlines and other control characters from a given key.
func SanitizeAPIKeyConfig(cfg Config, key string) {
	if !cfg.IsKnown(key) {
		return
	}
	cfg.Set(key, strings.TrimSpace(cfg.GetString(key)))
}
