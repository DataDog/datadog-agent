// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package lite extracts core config values (api_key, site, dd_url) from
// environment variables and config files without importing the full agent config
package lite

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const DefaultSite = "datadoghq.com"

// Source indicates where a config value was resolved from
type Source string

const (
	SourceEnv     Source = "env"
	SourceFile    Source = "file"
	SourceDefault Source = "default"
	SourceNone    Source = "none"
)

// ConfigField holds a resolved config value along with related info
type ConfigField struct {
	Value      string
	Source     Source
	MatchedKey string // MatchedKey records the original key text when resolved via fuzzy matching
}

// LiteConfig holds the minimal configuration extracted for Agent Health
type LiteConfig struct {
	APIKey         ConfigField
	Site           ConfigField
	DDURL          ConfigField
	ConfigFilePath string
	FileReadErr    error
}

// configKey defines a target config key with its exact regex pattern,
// fuzzy matching threshold, and precomputed separator-stripped form.
type configKey struct {
	name         string
	pattern      *regexp.Regexp
	maxFuzzyDist int
	strippedName string // precomputed stripSeparators(name)
}

// configKeys is the single source of truth for all target keys
// Both exact regex and fuzzy matching are driven from this list
var configKeys = []configKey{
	{"api_key", regexp.MustCompile(`(?m)^api_key:[ \t]+(.+?)[ \t]*(?:#.*)?$`), 2, "apikey"},
	{"site", regexp.MustCompile(`(?m)^site:[ \t]+(.+?)[ \t]*(?:#.*)?$`), 1, "site"},
	{"dd_url", regexp.MustCompile(`(?m)^dd_url:[ \t]+(.+?)[ \t]*(?:#.*)?$`), 2, "ddurl"},
}

// fields returns pointers to the config fields in the same order as configKeys.
func (cfg *LiteConfig) fields() []*ConfigField {
	return []*ConfigField{&cfg.APIKey, &cfg.Site, &cfg.DDURL}
}

// Extract returns a LiteConfig
func Extract(cliConfPath, defaultConfPath string) LiteConfig {
	cfg := LiteConfig{
		APIKey: ConfigField{Source: SourceNone},
		Site:   ConfigField{Source: SourceNone},
		DDURL:  ConfigField{Source: SourceNone},
	}

	extractFromEnv(&cfg)
	extractFromFile(&cfg, cliConfPath, defaultConfPath)

	// defaults
	if cfg.Site.Source == SourceNone {
		cfg.Site.Value = DefaultSite
		cfg.Site.Source = SourceDefault
	}

	return cfg
}

// extractFromEnv gets fields from environment variables (highest priority)
// env var bindings match pkg/config/setup BindEnv order.
func extractFromEnv(cfg *LiteConfig) {
	setFromEnv := func(field *ConfigField, envVars ...string) {
		for _, env := range envVars {
			if v := os.Getenv(env); v != "" {
				field.Value = v
				field.Source = SourceEnv
				return
			}
		}
	}

	setFromEnv(&cfg.APIKey, "DD_API_KEY")
	setFromEnv(&cfg.Site, "DD_SITE")
	setFromEnv(&cfg.DDURL, "DD_DD_URL", "DD_URL")
}

// extractFromFile reads the config file and tries to get unresolved fields
// via exact regex matching, then fuzzy fallback
func extractFromFile(cfg *LiteConfig, cliConfPath, defaultConfPath string) {
	// resolve config file path <cliConfPath, defaultConfPath>
	// if a path doesn't end in .yaml/.yml, /datadog.yaml is appended
	var path string
	for _, p := range []string{cliConfPath, defaultConfPath} {
		if p == "" {
			continue
		}
		candidate := p
		if !strings.HasSuffix(candidate, ".yaml") && !strings.HasSuffix(candidate, ".yml") {
			candidate = filepath.Join(candidate, "datadog.yaml")
		}
		if _, err := os.Stat(candidate); err == nil {
			path = candidate
			break
		}
	}
	if path == "" {
		return
	}
	cfg.ConfigFilePath = path

	data, err := os.ReadFile(path)
	if err != nil {
		cfg.FileReadErr = err
		return
	}

	content := string(bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF}))

	fields := cfg.fields()
	anyUnresolved := false
	for i, ck := range configKeys {
		if fields[i].Source != SourceNone {
			continue
		}
		if m := ck.pattern.FindStringSubmatch(content); m != nil {
			if v := cleanValue(m[1]); v != "" {
				fields[i].Value = v
				fields[i].Source = SourceFile
				continue
			}
		}
		anyUnresolved = true
	}

	if anyUnresolved {
		fuzzyExtract(fields, content)
	}
}
