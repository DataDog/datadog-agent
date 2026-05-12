// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import "regexp"

// regexBindings drives the Tier-4 column-0 regex. The (?m) flag makes ^ match
// at the start of every line, so nested `api_key:` inside additional_endpoints
// or logs_config never gets surfaced as the primary credential.
var regexBindings = []struct {
	field   func(*Config) *ConfigField
	name    string
	pattern *regexp.Regexp
}{
	{func(c *Config) *ConfigField { return &c.APIKey }, "api_key",
		regexp.MustCompile(`(?m)^api_key:[ \t]+(.+?)[ \t]*(?:#.*)?$`)},
	{func(c *Config) *ConfigField { return &c.Site }, "site",
		regexp.MustCompile(`(?m)^site:[ \t]+(.+?)[ \t]*(?:#.*)?$`)},
	{func(c *Config) *ConfigField { return &c.DDURL }, "dd_url",
		regexp.MustCompile(`(?m)^dd_url:[ \t]+(.+?)[ \t]*(?:#.*)?$`)},
	{func(c *Config) *ConfigField { return &c.SecretBackendCommand }, "secret_backend_command",
		regexp.MustCompile(`(?m)^secret_backend_command:[ \t]+(.+?)[ \t]*(?:#.*)?$`)},
}

func applyRegex(cfg *Config, raw []byte) {
	for _, b := range regexBindings {
		f := b.field(cfg)
		if f.resolved() {
			continue
		}
		m := b.pattern.FindSubmatch(raw)
		if m == nil {
			continue
		}
		val := cleanValue(string(m[1]))
		if val == "" {
			continue
		}
		f.Value = val
		f.Source = SourceFileRegex
		f.MatchedKey = b.name
	}
}
