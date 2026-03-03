// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lite

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const DefaultSite = "datadoghq.com"

// Source indicates where a config value was resolved from.
type Source string

const (
	SourceEnv     Source = "env"
	SourceFile    Source = "file"
	SourceDefault Source = "default"
	SourceNone    Source = "none"
)

// ConfigField holds a resolved config value along with its provenance.
type ConfigField struct {
	Value      string
	Source     Source
	MatchedKey string // MatchedKey records the original key text when resolved via fuzzy matching
}

// LiteConfig holds the minimal configuration extracted for Agent Health.
type LiteConfig struct {
	APIKey         ConfigField
	Site           ConfigField
	DDURL          ConfigField
	ConfigFilePath string
	FileReadErr    error
}

// keyPatterns matches top-level YAML keys at column 0
var keyPatterns = map[string]*regexp.Regexp{
	"api_key": regexp.MustCompile(`(?m)^api_key:[ \t]+(.+?)[ \t]*(?:#.*)?$`),
	"site":    regexp.MustCompile(`(?m)^site:[ \t]+(.+?)[ \t]*(?:#.*)?$`),
	"dd_url":  regexp.MustCompile(`(?m)^dd_url:[ \t]+(.+?)[ \t]*(?:#.*)?$`),
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

	setFromFile := func(field *ConfigField, key string) {
		if field.Source != SourceNone {
			return
		}
		if m := keyPatterns[key].FindStringSubmatch(content); m != nil {
			if v := cleanValue(m[1]); v != "" {
				field.Value = v
				field.Source = SourceFile
			}
		}
	}

	setFromFile(&cfg.APIKey, "api_key")
	setFromFile(&cfg.Site, "site")
	setFromFile(&cfg.DDURL, "dd_url")

	// fuzzy fallback
	if cfg.APIKey.Source == SourceNone || cfg.Site.Source == SourceNone || cfg.DDURL.Source == SourceNone {
		fuzzyExtract(cfg, content)
	}
}
