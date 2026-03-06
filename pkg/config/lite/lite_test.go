// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package lite

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		t.Setenv(k, v)
	}
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	err := os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte(content), 0644)
	require.NoError(t, err)
	return dir
}

func TestExtractFromEnv(t *testing.T) {
	withEnv(t, map[string]string{
		"DD_API_KEY": "env-key",
		"DD_SITE":    "env-site.com",
		"DD_DD_URL":  "https://env-url.com",
		"DD_URL":     "https://fallback-url.com", // should lose to DD_DD_URL
	})

	cfg := Extract("", "")
	assert.Equal(t, "env-key", cfg.APIKey.Value)
	assert.Equal(t, SourceEnv, cfg.APIKey.Source)
	assert.Equal(t, "env-site.com", cfg.Site.Value)
	assert.Equal(t, SourceEnv, cfg.Site.Source)
	assert.Equal(t, "https://env-url.com", cfg.DDURL.Value)
	assert.Equal(t, SourceEnv, cfg.DDURL.Source)
}

func TestDDURLFallback(t *testing.T) {
	withEnv(t, map[string]string{"DD_URL": "https://dd-url.com"})
	cfg := Extract("", "")
	assert.Equal(t, "https://dd-url.com", cfg.DDURL.Value)
}

func TestExtractFromFile(t *testing.T) {
	dir := writeConfig(t, `
api_key: file-key-123
site: file-site.com
dd_url: https://file-url.com
`)
	cfg := Extract("", dir)
	assert.Equal(t, ConfigField{"file-key-123", SourceFile, ""}, cfg.APIKey)
	assert.Equal(t, ConfigField{"file-site.com", SourceFile, ""}, cfg.Site)
	assert.Equal(t, ConfigField{"https://file-url.com", SourceFile, ""}, cfg.DDURL)
	assert.Equal(t, filepath.Join(dir, "datadog.yaml"), cfg.ConfigFilePath)
	assert.NoError(t, cfg.FileReadErr)
}

func TestEnvOverridesFile(t *testing.T) {
	dir := writeConfig(t, "api_key: file-key\nsite: file-site.com\n")
	withEnv(t, map[string]string{"DD_API_KEY": "env-key"})

	cfg := Extract("", dir)
	assert.Equal(t, "env-key", cfg.APIKey.Value)
	assert.Equal(t, SourceEnv, cfg.APIKey.Source)
	assert.Equal(t, "file-site.com", cfg.Site.Value)
	assert.Equal(t, SourceFile, cfg.Site.Source)
}

func TestDefaults(t *testing.T) {
	cfg := Extract("", "")
	assert.Equal(t, DefaultSite, cfg.Site.Value)
	assert.Equal(t, SourceDefault, cfg.Site.Source)
	assert.Empty(t, cfg.APIKey.Value)
	assert.Equal(t, SourceNone, cfg.APIKey.Source)
	assert.Empty(t, cfg.DDURL.Value)
	assert.Equal(t, SourceNone, cfg.DDURL.Source)
}

func TestValueCleaning(t *testing.T) {
	tests := []struct {
		name, content, want string
	}{
		{"double quotes", `api_key: "quoted-key"`, "quoted-key"},
		{"single quotes", `api_key: 'quoted-key'`, "quoted-key"},
		{"trailing comment", "api_key: abc123 # comment", "abc123"},
		{"ENC secret", "api_key: ENC[secret-handle]", "ENC[secret-handle]"},
		{"empty value", "api_key:", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeConfig(t, tt.content)
			cfg := Extract("", dir)
			assert.Equal(t, tt.want, cfg.APIKey.Value)
		})
	}
}

func TestSkippedLines(t *testing.T) {
	tests := []struct {
		name, content string
	}{
		{"commented", "# api_key: commented-key"},
		{"indented", "some_section:\n  api_key: nested-key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeConfig(t, tt.content)
			cfg := Extract("", dir)
			assert.Equal(t, SourceNone, cfg.APIKey.Source)
		})
	}
}

func TestInvalidYAMLWithValidKeys(t *testing.T) {
	dir := writeConfig(t, `api_key: still-works
	broken: [yaml
  tabs:	mixed
another_key
  : bad indent
site: also-works.com
`)
	cfg := Extract("", dir)
	assert.Equal(t, "still-works", cfg.APIKey.Value)
	assert.Equal(t, "also-works.com", cfg.Site.Value)
}

func TestPathResolution(t *testing.T) {
	t.Run("explicit yaml file", func(t *testing.T) {
		dir := t.TempDir()
		yamlPath := filepath.Join(dir, "custom.yaml")
		require.NoError(t, os.WriteFile(yamlPath, []byte(`api_key: k`), 0644))
		cfg := Extract(yamlPath, "")
		assert.Equal(t, yamlPath, cfg.ConfigFilePath)
	})

	t.Run("yml extension", func(t *testing.T) {
		dir := t.TempDir()
		ymlPath := filepath.Join(dir, "config.yml")
		require.NoError(t, os.WriteFile(ymlPath, []byte(`api_key: k`), 0644))
		cfg := Extract(ymlPath, "")
		assert.Equal(t, ymlPath, cfg.ConfigFilePath)
	})

	t.Run("CLI path before default", func(t *testing.T) {
		cliDir := writeConfig(t, `api_key: cli-key`)
		defaultDir := writeConfig(t, `api_key: default-key`)
		cfg := Extract(cliDir, defaultDir)
		assert.Equal(t, "cli-key", cfg.APIKey.Value)
	})

	t.Run("nonexistent paths", func(t *testing.T) {
		cfg := Extract("/nonexistent", "/also/nonexistent")
		assert.Empty(t, cfg.ConfigFilePath)
		assert.NoError(t, cfg.FileReadErr)
	})

	t.Run("unreadable file", func(t *testing.T) {
		dir := t.TempDir()
		yamlPath := filepath.Join(dir, "datadog.yaml")
		require.NoError(t, os.WriteFile(yamlPath, []byte(`api_key: secret`), 0644))
		require.NoError(t, os.Chmod(yamlPath, 0000))
		t.Cleanup(func() { os.Chmod(yamlPath, 0644) }) //nolint:errcheck

		cfg := Extract("", dir)
		assert.Error(t, cfg.FileReadErr)
		assert.Equal(t, SourceNone, cfg.APIKey.Source)
	})
}

func TestPlatformEdgeCases(t *testing.T) {
	t.Run("BOM stripping", func(t *testing.T) {
		dir := t.TempDir()
		content := append([]byte{0xEF, 0xBB, 0xBF}, []byte("api_key: bom-key\n")...)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), content, 0644))
		cfg := Extract("", dir)
		assert.Equal(t, "bom-key", cfg.APIKey.Value)
	})

	t.Run("Windows line endings", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, "datadog.yaml"),
			[]byte("api_key: win-key\r\nsite: win-site.com\r\n"), 0644))
		cfg := Extract("", dir)
		assert.Equal(t, "win-key", cfg.APIKey.Value)
		assert.Equal(t, "win-site.com", cfg.Site.Value)
	})
}

func TestDamerauLevenshtein(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want int
	}{
		{"insertion", "api_key", "api_keys", 1},
		{"deletion", "api_key", "ap_key", 1},
		{"substitution", "api_key", "abi_key", 1},
		{"transposition", "site", "stie", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, damerauLevenshtein(tt.a, tt.b))
		})
	}
}

func TestFuzzyMatching(t *testing.T) {
	apiKey := func(c LiteConfig) ConfigField { return c.APIKey }
	site := func(c LiteConfig) ConfigField { return c.Site }

	tests := []struct {
		name  string
		input string
		get   func(LiteConfig) ConfigField
		want  ConfigField
	}{
		{"substitution", "abi_key: abc123", apiKey, ConfigField{"abc123", SourceFile, "abi_key"}},
		{"case insensitive", "Api_Key: abc123", apiKey, ConfigField{"abc123", SourceFile, "Api_Key"}},
		{"wrong separator", "api_key; abc123", apiKey, ConfigField{"abc123", SourceFile, "api_key"}},
		{"missing underscore", "apikey: abc123", apiKey, ConfigField{"abc123", SourceFile, "apikey"}},
		{"transposition", "stie: datadoghq.com", site, ConfigField{"datadoghq.com", SourceFile, "stie"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeConfig(t, tt.input)
			cfg := Extract("", dir)
			assert.Equal(t, tt.want, tt.get(cfg))
		})
	}
}

func TestFuzzyNoMatch(t *testing.T) {
	tests := []struct {
		name, input string
	}{
		{"denylist app_key", "app_key: xyz"},
		{"unrelated key", "skip_ssl_validation: true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := writeConfig(t, tt.input)
			cfg := Extract("", dir)
			assert.Empty(t, cfg.APIKey.MatchedKey)
			assert.Empty(t, cfg.Site.MatchedKey)
			assert.Empty(t, cfg.DDURL.MatchedKey)
		})
	}
}

func TestExactTakesPriorityOverFuzzy(t *testing.T) {
	dir := writeConfig(t, "api_key: exact-value\nabi_key: fuzzy-value\n")
	cfg := Extract("", dir)
	assert.Equal(t, "exact-value", cfg.APIKey.Value)
	assert.Empty(t, cfg.APIKey.MatchedKey)
}

func TestFuzzyMultipleTypos(t *testing.T) {
	dir := writeConfig(t, "abi_key: key123\nssite: mysite.com\ndd_urls: https://custom.com\n")
	cfg := Extract("", dir)
	assert.Equal(t, ConfigField{"key123", SourceFile, "abi_key"}, cfg.APIKey)
	assert.Equal(t, ConfigField{"mysite.com", SourceFile, "ssite"}, cfg.Site)
	assert.Equal(t, ConfigField{"https://custom.com", SourceFile, "dd_urls"}, cfg.DDURL)
}
