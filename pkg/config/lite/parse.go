// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import (
	"strings"

	"go.yaml.in/yaml/v3"
)

// applyFullYAML is the Tier-2 strategy. It runs yaml.Unmarshal on the entire
// file and, on success, populates any unresolved top-level field from the map.
// On failure it records the error on cfg.YAMLParseErr and leaves the fields
// alone — the next tier will try.
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

// applyTopLevelYAML is the Tier-3 strategy. It drops any line that begins with
// whitespace so deeply-nested blocks that fail YAML parsing get pruned, and
// then re-runs yaml.Unmarshal on the surviving top-level mapping. This rescues
// the common "broken nested block" case where api_key/site are fine at the
// top level but `process_config:` further down is malformed.
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

// stripIndented keeps lines that start at column 0 and removes everything
// else. Commented lines and blank lines are preserved (yaml handles them).
// Lines starting with a tab are dropped too — yaml treats them as invalid
// indentation anyway.
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

// applyFromMap copies values from a successfully-parsed top-level map into
// the not-yet-resolved fields. We intentionally only look at top-level keys:
// nested *_api_key / additional_endpoints / logs_config.api_key fields are
// auxiliary and would confuse lite-mode if surfaced as the primary credential.
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

// stringOrEmpty coerces a yaml.Unmarshal'd interface{} into a string. YAML
// often produces int / bool for typo'd values; we only care about strings.
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
