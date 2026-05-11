// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import (
	"strings"

	"go.yaml.in/yaml/v3"
)

// applyFullYAML is the Tier-2 strategy: yaml.Unmarshal of the entire file.
// On failure it records cfg.YAMLParseErr and leaves the fields alone.
func applyFullYAML(cfg *LiteConfig, raw []byte) {
	if len(raw) == 0 {
		return
	}
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		cfg.YAMLParseErr = err
		return
	}
	cfg.YAMLParseErr = nil
	cfg.ParsedConfig = m
	applyFromMap(cfg, m, SourceFileYAMLFull)
}

// applyTopLevelYAML is the Tier-3 strategy: drop every indented line, then
// re-run yaml.Unmarshal. This rescues the common "broken nested block" case
// where top-level fields are fine but a sub-block like `process_config:`
// further down is malformed.
func applyTopLevelYAML(cfg *LiteConfig, raw []byte) {
	stripped := stripIndented(raw)
	if len(stripped) == 0 {
		return
	}
	var m map[string]any
	if err := yaml.Unmarshal(stripped, &m); err != nil {
		return
	}
	applyFromMap(cfg, m, SourceFileYAMLTop)
}

func stripIndented(raw []byte) []byte {
	var b strings.Builder
	b.Grow(len(raw))
	for line := range strings.SplitSeq(string(raw), "\n") {
		if len(line) == 0 {
			b.WriteByte('\n')
			continue
		}
		c := line[0]
		if c == ' ' || c == '\t' {
			b.WriteByte('\n')
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

// applyFromMap copies top-level values into unresolved fields. Nested
// *_api_key / additional_endpoints / logs_config.api_key are intentionally
// ignored — they are auxiliary and would confuse lite-mode if promoted.
func applyFromMap(cfg *LiteConfig, m map[string]any, src Source) {
	set := func(field *ConfigField, key string) {
		if field.resolved() {
			return
		}
		raw, ok := m[key]
		if !ok {
			return
		}
		s := stringOrEmpty(raw)
		if s == "" {
			return
		}
		field.Value = s
		field.Source = src
		field.MatchedKey = key
	}
	set(&cfg.APIKey, "api_key")
	set(&cfg.Site, "site")
	set(&cfg.DDURL, "dd_url")
	set(&cfg.SecretBackendCommand, "secret_backend_command")
}

// stringOrEmpty coerces a yaml.Unmarshal'd value into a string. Typo'd values
// often come back as int / bool — we only accept strings.
func stringOrEmpty(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case []byte:
		return string(x)
	default:
		return ""
	}
}
