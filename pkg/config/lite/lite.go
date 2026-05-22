// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
)

// Extract runs the tiered resolver pipeline against the process environment
// and the candidate datadog.yaml paths. cliConfPath is from `--cfgpath` (or
// empty); defaultConfPath is the platform-specific default. The first path
// that exists is used.
func Extract(ctx context.Context, cliConfPath, defaultConfPath string) LiteConfig {
	cfg := LiteConfig{
		APIKey:               ConfigField{Source: SourceNone},
		Site:                 ConfigField{Source: SourceNone},
		DDURL:                ConfigField{Source: SourceNone},
		SecretBackendCommand: ConfigField{Source: SourceNone},
	}

	applyEnv(&cfg)

	// Pick the first existing datadog.yaml. A path without a .yaml/.yml
	// suffix is treated as a directory.
	for _, p := range []string{cliConfPath, defaultConfPath} {
		if p == "" {
			continue
		}
		if !strings.HasSuffix(p, ".yaml") && !strings.HasSuffix(p, ".yml") {
			p = filepath.Join(p, "datadog.yaml")
		}
		if _, err := os.Stat(p); err == nil {
			cfg.ConfigFilePath = p
			break
		}
	}

	var raw []byte
	if cfg.ConfigFilePath != "" {
		if data, err := os.ReadFile(cfg.ConfigFilePath); err != nil {
			cfg.FileReadErr = err
		} else {
			raw = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}) // strip UTF-8 BOM
		}
	}

	if len(raw) > 0 {
		// applyFullYAML always runs to capture YAMLParseErr / ParsedConfig.
		applyFullYAML(&cfg, raw)
		for _, apply := range []func(*LiteConfig, []byte){applyTopLevelYAML, applyRegex, applyFuzzy} {
			// SecretBackendCommand is excluded: it only exists to feed resolveENC.
			if cfg.APIKey.resolved() && cfg.Site.resolved() && cfg.DDURL.resolved() {
				break
			}
			apply(&cfg, raw)
		}
	}

	// ENC[] resolution runs before defaults so an unresolvable encrypted
	// credential stays unresolved and never wins over a fallback. No-ops when
	// SecretBackendCommand is empty.
	if ctx == nil {
		ctx = context.Background()
	}
	resolveENC(ctx, &cfg)

	if cfg.Site.Source == SourceNone {
		cfg.Site.Value = DefaultSite
		cfg.Site.Source = SourceDefault
	}
	return cfg
}
