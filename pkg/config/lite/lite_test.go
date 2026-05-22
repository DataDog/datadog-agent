// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package lite

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withYAML writes a datadog.yaml into a t.TempDir() and returns the dir path,
// suitable to pass to Extract() as defaultConfPath.
func withYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), []byte(content), 0o600))
	return dir
}

// extract runs Extract with a TODO context — the test suite never configures
// a secret_backend_command, so the resolver no-ops.
func extract(cliPath, defaultPath string) LiteConfig {
	return Extract(context.TODO(), cliPath, defaultPath)
}

func TestEnvBeatsFile(t *testing.T) {
	dir := withYAML(t, "api_key: file_key\nsite: file_site.com\n")
	t.Setenv("DD_API_KEY", "env_key")
	t.Setenv("DD_SITE", "env_site.com")

	cfg := extract("", dir)
	assert.Equal(t, "env_key", cfg.APIKey.Value)
	assert.Equal(t, SourceEnv, cfg.APIKey.Source)
	assert.Equal(t, "env_site.com", cfg.Site.Value)
	assert.Equal(t, SourceEnv, cfg.Site.Source)
}

func TestDDURLEnvPriority(t *testing.T) {
	t.Setenv("DD_DD_URL", "https://primary.example")
	t.Setenv("DD_URL", "https://legacy.example") // should lose
	cfg := extract("", "")
	assert.Equal(t, "https://primary.example", cfg.DDURL.Value)
	assert.Equal(t, SourceEnv, cfg.DDURL.Source)
}

func TestDDURLLegacyFallback(t *testing.T) {
	t.Setenv("DD_URL", "https://legacy.example")
	cfg := extract("", "")
	assert.Equal(t, "https://legacy.example", cfg.DDURL.Value)
}

func TestTier2_FullYAML(t *testing.T) {
	dir := withYAML(t, "api_key: abc123\nsite: datadoghq.eu\ndd_url: https://example.com\n")
	cfg := extract("", dir)
	assert.Equal(t, "abc123", cfg.APIKey.Value)
	assert.Equal(t, SourceFileYAMLFull, cfg.APIKey.Source)
	assert.Equal(t, "datadoghq.eu", cfg.Site.Value)
	assert.Equal(t, SourceFileYAMLFull, cfg.Site.Source)
	assert.Equal(t, "https://example.com", cfg.DDURL.Value)
	assert.NotNil(t, cfg.ParsedConfig, "Tier 2 success should leave parsed map for downstream")
	assert.Equal(t, "abc123", cfg.ParsedConfig["api_key"])
	assert.NoError(t, cfg.YAMLParseErr)
}

func TestTier2_TopLevelIgnoresNestedAPIKey(t *testing.T) {
	dir := withYAML(t, `
api_key: top_level_wins
additional_endpoints:
  https://other.dd:
    - nested_key_1
    - nested_key_2
logs_config:
  api_key: nested_logs_key
`)
	cfg := extract("", dir)
	assert.Equal(t, "top_level_wins", cfg.APIKey.Value,
		"Lite mode must never surface a nested api_key — only top-level")
}

func TestTier3_StripIndentedRescuesBrokenNested(t *testing.T) {
	// Top-level api_key/site are fine, but process_config block has bad YAML
	// — full parse fails, top-level-only parse succeeds.
	dir := withYAML(t, `
api_key: rescued_key
site: datadoghq.eu
process_config:
  enabled: "true"
  scrub_args: [unterminated
  more_broken: {missing
`)
	cfg := extract("", dir)
	assert.Equal(t, "rescued_key", cfg.APIKey.Value)
	assert.Equal(t, SourceFileYAMLTop, cfg.APIKey.Source)
	assert.Equal(t, "datadoghq.eu", cfg.Site.Value)
	assert.Error(t, cfg.YAMLParseErr, "Tier 2 should have failed")
}

func TestTier4_RegexWhenYAMLCompletelyBroken(t *testing.T) {
	// File so broken that both Tier 2 and Tier 3 fail (the first line is
	// malformed). Regex must still find the top-level api_key.
	dir := withYAML(t, "{ this is not yaml at all\napi_key: still_works\nsite: dd.eu\n}}}\n")
	cfg := extract("", dir)
	assert.Equal(t, "still_works", cfg.APIKey.Value)
	assert.Equal(t, SourceFileRegex, cfg.APIKey.Source)
	assert.Equal(t, "api_key", cfg.APIKey.MatchedKey)
}

func TestTier4_RegexSkipsCommentedLines(t *testing.T) {
	dir := withYAML(t, "# api_key: this_is_in_a_comment\napi_key: real_key\n")
	cfg := extract("", dir)
	assert.Equal(t, "real_key", cfg.APIKey.Value)
}

func TestTier4_RegexSkipsIndentedLines(t *testing.T) {
	// YAML too broken for Tier 2/3 to parse. The only `api_key:` line is
	// indented under a (broken) parent — regex must NOT match it. Want
	// SourceNone, not a nested key surfaced as primary.
	dir := withYAML(t, "{ broken yaml at top level\nadditional_endpoints:\n  api_key: nested_should_be_ignored\n")
	cfg := extract("", dir)
	assert.Equal(t, SourceNone, cfg.APIKey.Source,
		"indented api_key under a broken parent must not be promoted")
}

func TestTier4_RegexCleansAnchorPrefix(t *testing.T) {
	dir := withYAML(t, "{ broken yaml\napi_key: &my_anchor abc123\n")
	cfg := extract("", dir)
	assert.Equal(t, "abc123", cfg.APIKey.Value,
		"Regex tier must strip the YAML anchor prefix from the captured value")
}

func TestTier4_RegexTrimsTrailingComment(t *testing.T) {
	dir := withYAML(t, "{ broken\napi_key: abc # this is the key\n")
	cfg := extract("", dir)
	assert.Equal(t, "abc", cfg.APIKey.Value)
}

func TestTier5_FuzzyCatchesTypos(t *testing.T) {
	cases := []struct {
		name, content, want string
	}{
		{"no underscore", "{broken\napikey: foo\n", "foo"},
		{"hyphen", "{broken\napi-key: foo\n", "foo"},
		{"PascalCase", "{broken\nApi_Key: foo\n", "foo"},
		{"substitution", "{broken\nabi_key: foo\n", "foo"},
		{"transposition (site→stie)", "{broken\nstie: dd.eu\n", "dd.eu"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := withYAML(t, c.content)
			cfg := extract("", dir)
			if strings.Contains(c.content, "stie:") {
				assert.Equal(t, c.want, cfg.Site.Value)
				assert.Equal(t, SourceFileFuzzy, cfg.Site.Source)
			} else {
				assert.Equal(t, c.want, cfg.APIKey.Value)
				assert.Equal(t, SourceFileFuzzy, cfg.APIKey.Source)
			}
		})
	}
}

func TestTier5_FuzzyCollects_AllCandidatesForAPIKey(t *testing.T) {
	// Both `app_key` and `api_kye` are distance 1 from "api_key". The fuzzy
	// tier should pick one as primary and stash the other for the rescue
	// path to retry on 401 — the intake decides which one is real.
	dir := withYAML(t, "{broken\napp_key: app_secret\napi_kye: api_secret\n")
	cfg := extract("", dir)
	assert.Equal(t, SourceFileFuzzy, cfg.APIKey.Source)
	require.Len(t, cfg.APIKeyCandidates, 1)
	values := []string{cfg.APIKey.Value, cfg.APIKeyCandidates[0].Value}
	assert.ElementsMatch(t, []string{"app_secret", "api_secret"}, values)
}

func TestTier5_FuzzySingleCandidate(t *testing.T) {
	// One distance-1 match, nothing else nearby — no extras to stash.
	dir := withYAML(t, "{broken\nabi_key: foo\n")
	cfg := extract("", dir)
	assert.Equal(t, "foo", cfg.APIKey.Value)
	assert.Empty(t, cfg.APIKeyCandidates)
}

func TestTier6_DefaultSite(t *testing.T) {
	cfg := extract("", "")
	assert.Equal(t, DefaultSite, cfg.Site.Value)
	assert.Equal(t, SourceDefault, cfg.Site.Source)
	assert.Equal(t, SourceNone, cfg.APIKey.Source, "no default for api_key")
	assert.Equal(t, SourceNone, cfg.DDURL.Source, "no default for dd_url")
}

func TestENC_WithoutBackendStaysEncrypted(t *testing.T) {
	dir := withYAML(t, "api_key: ENC[some_handle]\nsite: datadoghq.eu\n")
	cfg := Extract(context.Background(), "", dir)
	assert.Equal(t, SourceEncrypted, cfg.APIKey.Source,
		"ENC[] without a secret_backend_command must be marked encrypted, not usable")
}

func TestENC_ResolvedViaSecretBackend(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake backend is a POSIX shell script; the exec path is platform-agnostic")
	}
	// Fake secret backend: tiny shell script that reads a JSON request on
	// stdin and writes a JSON response on stdout.
	dir := t.TempDir()
	backend := filepath.Join(dir, "fake_backend.sh")
	const script = `#!/bin/sh
cat > /dev/null
printf '%s' '{"ENC[the_handle]":{"value":"resolved_value","error":""}}'
`
	require.NoError(t, os.WriteFile(backend, []byte(script), 0o700))

	yamlFile := filepath.Join(dir, "datadog.yaml")
	require.NoError(t, os.WriteFile(yamlFile,
		[]byte("api_key: ENC[the_handle]\nsecret_backend_command: "+backend+"\nsite: datadoghq.eu\n"),
		0o600))

	cfg := Extract(context.Background(), "", dir)
	assert.Equal(t, "resolved_value", cfg.APIKey.Value)
	assert.Equal(t, SourceSecretBackend, cfg.APIKey.Source)
}

func TestNoLiteConfigFileAtAll(t *testing.T) {
	cfg := extract("/nonexistent", "/also/nonexistent")
	assert.Empty(t, cfg.ConfigFilePath)
	assert.NoError(t, cfg.FileReadErr)
	assert.Equal(t, DefaultSite, cfg.Site.Value)
}

func TestExplicitCLIPathBeatsDefault(t *testing.T) {
	cli := withYAML(t, "api_key: cli_key\n")
	def := withYAML(t, "api_key: default_key\n")
	cfg := extract(cli, def)
	assert.Equal(t, "cli_key", cfg.APIKey.Value)
}

func TestQuotedAndCRLFAreCleaned(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"),
		[]byte("api_key: \"quoted\"\r\nsite: 'singled'\r\n"), 0o600))
	cfg := extract("", dir)
	assert.Equal(t, "quoted", cfg.APIKey.Value)
	assert.Equal(t, "singled", cfg.Site.Value)
}

func TestBOMIsStripped(t *testing.T) {
	dir := t.TempDir()
	content := append([]byte{0xEF, 0xBB, 0xBF}, []byte("api_key: bom_key\n")...)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "datadog.yaml"), content, 0o600))
	cfg := extract("", dir)
	assert.Equal(t, "bom_key", cfg.APIKey.Value)
}

func TestEmptyValueIsSkipped(t *testing.T) {
	dir := withYAML(t, "api_key:\nsite: dd.eu\n")
	cfg := extract("", dir)
	assert.Equal(t, SourceNone, cfg.APIKey.Source)
	assert.Equal(t, "dd.eu", cfg.Site.Value)
}

func TestDamerauLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"api_key", "api_key", 0},
		{"api_key", "apikey", 1},  // deletion
		{"api_key", "abi_key", 1}, // substitution
		{"site", "stie", 1},       // transposition
		{"foo", "bar", 3},         // 3 substitutions
		{"", "abc", 3},            // pure insertion
	}
	for _, c := range cases {
		assert.Equal(t, c.want, damerauLevenshtein(c.a, c.b), "%s vs %s", c.a, c.b)
	}
}

func TestStripAnchor(t *testing.T) {
	assert.Equal(t, "value", stripAnchor("&anchor value"))
	assert.Equal(t, "value", stripAnchor("*alias value"))
	assert.Equal(t, "& not_really_an_anchor", stripAnchor("& not_really_an_anchor"))
	assert.Equal(t, "plain", stripAnchor("plain"))
}

func TestCleanValueStripsAnchors(t *testing.T) {
	assert.Equal(t, "abc123", cleanValue("&my_anchor abc123"))
	assert.Equal(t, "abc123", cleanValue("*alias abc123"))
	assert.Equal(t, "abc", cleanValue("\"abc\""))
	assert.Equal(t, "abc", cleanValue("'abc'"))
	assert.Equal(t, "abc", cleanValue("abc\r"))
}
