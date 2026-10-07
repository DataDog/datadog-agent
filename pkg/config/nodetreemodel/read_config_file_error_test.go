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

func TestValidConfigFileRecordsNoParseWarning(t *testing.T) {
	conf, _ := newFileConfig(t, "api_key: abc123\nsite: datadoghq.eu\n")

	require.NoError(t, conf.ReadInConfig())

	assert.Equal(t, "abc123", conf.GetString("api_key"))
	assert.Equal(t, "datadoghq.eu", conf.GetString("site"))
	assert.Empty(t, conf.Warnings())
}
