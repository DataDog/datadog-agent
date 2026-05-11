// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import "context"

// Extract runs the tiered resolver pipeline against the process environment
// and the candidate datadog.yaml paths. cliConfPath is from `--cfgpath` (or
// empty); defaultConfPath is the platform-specific default. The first path
// that exists is used. Pass a real ctx to enable ENC[] resolution via
// secret_backend_command; a nil ctx skips that step.
func Extract(ctx context.Context, cliConfPath, defaultConfPath string) LiteConfig {
	cfg := LiteConfig{
		APIKey:               ConfigField{Source: SourceNone},
		Site:                 ConfigField{Source: SourceNone},
		DDURL:                ConfigField{Source: SourceNone},
		SecretBackendCommand: ConfigField{Source: SourceNone},
	}

	applyEnv(&cfg)

	cfg.ConfigFilePath = resolveConfigPath(cliConfPath, defaultConfPath)
	var raw []byte
	if cfg.ConfigFilePath != "" {
		var err error
		if raw, err = readConfigFile(cfg.ConfigFilePath); err != nil {
			cfg.FileReadErr = err
		}
	}

	if len(raw) > 0 {
		// applyFullYAML always runs to capture YAMLParseErr / ParsedConfig.
		applyFullYAML(&cfg, raw)
		for _, apply := range []func(*LiteConfig, []byte){applyTopLevelYAML, applyRegex, applyFuzzy} {
			if allResolved(&cfg) {
				break
			}
			apply(&cfg, raw)
		}
	}

	// ENC[] resolution runs before defaults so an unresolvable encrypted
	// credential stays unresolved and never wins over a fallback.
	if ctx != nil {
		resolveENC(ctx, &cfg)
	}

	applyDefaults(&cfg)
	return cfg
}

// allResolved reports whether the three credential fields have all settled.
// SecretBackendCommand is excluded: it only exists to feed resolveENC.
func allResolved(cfg *LiteConfig) bool {
	return cfg.APIKey.resolved() && cfg.Site.resolved() && cfg.DDURL.resolved()
}

func applyDefaults(cfg *LiteConfig) {
	if cfg.Site.Source == SourceNone {
		cfg.Site.Value = DefaultSite
		cfg.Site.Source = SourceDefault
	}
}
