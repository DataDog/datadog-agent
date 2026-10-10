// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/model"
)

// newFileConfig writes content to a datadog.yaml in a temp dir and returns a
// config pointed at it, so tests exercise readInConfig rather than ReadConfig.
func newFileConfig(t *testing.T, content string) (model.BuildableConfig, string) {
	t.Helper()

	path := filepath.Join(t.TempDir(), "datadog.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	conf := NewNodeTreeConfig("datadog", "DD", nil)
	conf.BindEnvAndSetDefault("api_key", "")
	conf.BindEnvAndSetDefault("site", "datadoghq.com")
	conf.BuildSchema()
	conf.SetConfigFile(path)

	return conf, path
}

// syntaxErrorYAML reproduces FRAGENT-3726: a value where a key is expected.
const syntaxErrorYAML = "api_key: abc123\nfoo: bar: baz\n"

// TestSyntaxErrorDiscardsWholeFile documents the behaviour that makes a broken
// datadog.yaml dangerous: every setting in it is silently ignored.
func TestSyntaxErrorDiscardsWholeFile(t *testing.T) {
	conf, _ := newFileConfig(t, syntaxErrorYAML)

	require.Error(t, conf.ReadInConfig())

	assert.Empty(t, conf.GetString("api_key"), "api_key from the file must not be applied")
	assert.Equal(t, "datadoghq.com", conf.GetString("site"), "defaults must be in use")
}

func TestSyntaxErrorIsRecordedAsWarning(t *testing.T) {
	conf, path := newFileConfig(t, syntaxErrorYAML)

	require.Error(t, conf.ReadInConfig())

	warnings := strings.Join(conf.Warnings(), "\n")
	assert.Contains(t, warnings, path, "warning must name the offending file")
	assert.Contains(t, warnings, "mapping values are not allowed",
		"warning must preserve the underlying parser error")
	assert.Contains(t, warnings, "ignored",
		"warning must say the file's settings are not in effect")
}

// TestDuplicateKeysAreRecordedAsWarning covers the strict-only failure class:
// the file still loads leniently, so the conflict is otherwise invisible.
func TestDuplicateKeysAreRecordedAsWarning(t *testing.T) {
	conf, path := newFileConfig(t, "api_key: first\napi_key: second\n")

	require.NoError(t, conf.ReadInConfig())
	assert.Equal(t, "second", conf.GetString("api_key"), "last duplicate wins")

	warnings := strings.Join(conf.Warnings(), "\n")
	assert.Contains(t, warnings, path, "warning must name the offending file")
	assert.Contains(t, warnings, "already set in map",
		"warning must preserve the underlying parser error")
}

// TestConfigFileErrorReportsParseFailure covers the accessor the status provider
// uses, so it does not have to string-match its way out of Warnings().
func TestConfigFileErrorReportsParseFailure(t *testing.T) {
	conf, path := newFileConfig(t, syntaxErrorYAML)

	require.Error(t, conf.ReadInConfig())

	assert.Contains(t, conf.ConfigFileError(), path)
	assert.Contains(t, conf.ConfigFileError(), "ignored")
}

func TestConfigFileErrorEmptyForValidFile(t *testing.T) {
	conf, _ := newFileConfig(t, "api_key: abc123\n")

	require.NoError(t, conf.ReadInConfig())

	assert.Empty(t, conf.ConfigFileError())
}

// TestConfigFileErrorScrubsSecrets guards the quoted context: the surrounding
// lines of a broken datadog.yaml routinely hold credentials, and the message is
// rendered by agent status and the Agent Manager.
func TestConfigFileErrorScrubsSecrets(t *testing.T) {
	const apiKey = "abcdef0123456789abcdef0123456789"

	// The syntax error is on line 3, so lines 1 and 2 land in the quoted context.
	conf, _ := newFileConfig(t, "api_key: "+apiKey+"\npassword: hunter2\nfoo: bar: baz\n")

	require.Error(t, conf.ReadInConfig())

	assert.NotContains(t, conf.ConfigFileError(), apiKey, "api_key must not be retained")
	assert.NotContains(t, conf.ConfigFileError(), "hunter2", "password must not be retained")

	warnings := strings.Join(conf.Warnings(), "\n")
	assert.NotContains(t, warnings, apiKey, "api_key must not reach flare via Warnings()")
	assert.NotContains(t, warnings, "hunter2")

	// The diagnostic must survive scrubbing, otherwise the message is useless.
	assert.Contains(t, conf.ConfigFileError(), "mapping values are not allowed")
	assert.Contains(t, conf.ConfigFileError(), "> 3 |")
}

// TestConfigFileErrorScrubsSecretsOnDuplicateKeys covers the strict-only class,
// where a duplicated api_key is exactly what gets quoted.
func TestConfigFileErrorScrubsSecretsOnDuplicateKeys(t *testing.T) {
	const apiKey = "abcdef0123456789abcdef0123456789"

	conf, _ := newFileConfig(t, "api_key: "+apiKey+"\napi_key: "+apiKey+"\n")

	require.NoError(t, conf.ReadInConfig())

	assert.NotContains(t, conf.ConfigFileError(), apiKey)
	assert.NotContains(t, strings.Join(conf.Warnings(), "\n"), apiKey)
}

// TestConfigFileErrorScrubsAnchoredSecrets covers the scrubber families whose
// regexes are anchored to ^\s* (matchYAMLKeyEnding: *_token, *_secret,
// *access_key, private_key). Prefixing a line with "  3 | " before scrubbing
// defeats them, so the context must be scrubbed while still raw YAML.
func TestConfigFileErrorScrubsAnchoredSecrets(t *testing.T) {
	// The syntax error is on line 3, so lines 1-5 land in the quoted context.
	conf, _ := newFileConfig(t, ""+
		"auth_token: plaintexttokenvalue\n"+
		"client_secret: supersecretvalue\n"+
		"foo: bar: baz\n"+
		"aws_access_key: accesskeyvalue\n"+
		"private_key: privatekeyvalue\n")

	require.Error(t, conf.ReadInConfig())

	got := conf.ConfigFileError()
	assert.NotContains(t, got, "plaintexttokenvalue", "auth_token must be redacted")
	assert.NotContains(t, got, "supersecretvalue", "client_secret must be redacted")
	assert.NotContains(t, got, "accesskeyvalue", "aws_access_key must be redacted")
	assert.NotContains(t, got, "privatekeyvalue", "private_key must be redacted")

	assert.NotContains(t, strings.Join(conf.Warnings(), "\n"), "plaintexttokenvalue")

	// Scrubbing must not shift the line numbering the marker depends on.
	assert.Contains(t, got, "> 3 | foo: bar: baz")
}

func TestValidConfigFileRecordsNoParseWarning(t *testing.T) {
	conf, _ := newFileConfig(t, "api_key: abc123\nsite: datadoghq.eu\n")

	require.NoError(t, conf.ReadInConfig())

	assert.Equal(t, "abc123", conf.GetString("api_key"))
	assert.Equal(t, "datadoghq.eu", conf.GetString("site"))
	assert.Empty(t, conf.Warnings())
}
