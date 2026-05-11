// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import "context"

// Extract runs the tiered resolver pipeline against the process environment
// and the candidate datadog.yaml paths. cliConfPath is the path supplied via
// `--cfgpath` (or empty); defaultConfPath is the platform-specific default
// such as /etc/datadog-agent. The first path that exists is used.
//
// Extract is read-only and side-effect-free apart from secret-backend
// resolution, which it skips when ctx is nil or no secret_backend_command was
// discovered. Pass a real ctx to enable ENC[] resolution.
func Extract(ctx context.Context, cliConfPath, defaultConfPath string) LiteConfig {
	cfg := LiteConfig{
		APIKey:               ConfigField{Source: SourceNone},
		Site:                 ConfigField{Source: SourceNone},
		DDURL:                ConfigField{Source: SourceNone},
		SecretBackendCommand: ConfigField{Source: SourceNone},
	}

	// Tier 1 — environment variables (highest priority).
	applyEnv(&cfg)

	// Locate the file once and read it; every file-based tier reuses raw.
	path := resolveConfigPath(cliConfPath, defaultConfPath)
	cfg.ConfigFilePath = path

	var raw []byte
	if path != "" {
		var err error
		raw, err = readConfigFile(path)
		if err != nil {
			cfg.FileReadErr = err
		}
	}

	// Tier 2 — full yaml.Unmarshal of the raw bytes.
	if len(raw) > 0 {
		applyFullYAML(&cfg, raw)
	}

	// Tier 3 — strip indented lines, retry yaml.Unmarshal.
	if len(raw) > 0 && !allResolved(&cfg) {
		applyTopLevelYAML(&cfg, raw)
	}

	// Tier 4 — column-0 anchored regex.
	if len(raw) > 0 && !allResolved(&cfg) {
		applyRegex(&cfg, raw)
	}

	// Tier 5 — Damerau-Levenshtein fuzzy match on top-level keys.
	if len(raw) > 0 && !allResolved(&cfg) {
		applyFuzzy(&cfg, raw)
	}

	// ENC[handle] resolution. Done before defaults so that an unresolvable
	// encrypted credential is treated as still-unresolved and never wins over
	// a later fallback.
	if ctx != nil {
		resolveENC(ctx, &cfg)
	}

	// Tier 6 — defaults (only site has one).
	applyDefaults(&cfg)

	return cfg
}

// allResolved reports whether the three credential fields have all settled.
// SecretBackendCommand is not counted: it only exists to feed resolveENC.
func allResolved(cfg *LiteConfig) bool {
	return cfg.APIKey.resolved() && cfg.Site.resolved() && cfg.DDURL.resolved()
}

func applyDefaults(cfg *LiteConfig) {
	if cfg.Site.Source == SourceNone {
		cfg.Site.Value = DefaultSite
		cfg.Site.Source = SourceDefault
	}
}
