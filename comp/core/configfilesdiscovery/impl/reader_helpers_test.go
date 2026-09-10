// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build docker || (cri && containerd)

package configfilesdiscoveryimpl

import (
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterEnvVars(t *testing.T) {
	selected := map[string]struct{}{
		"EMPTY":          {},
		"MISSING":        {},
		"REDIS_PASSWORD": {},
		"REDIS_PORT":     {},
		"WITH_EQUALS":    {},
	}

	env := filterEnvVars([]string{
		"REDIS_PORT=6379",
		"MALFORMED",
		"WITH_EQUALS=a=b=c",
		"EMPTY=",
		"REDIS_PORT=6380",
		"REDIS_PASSWORD=secret",
		"UNREQUESTED=value",
	}, func(name string) bool {
		_, ok := selected[name]
		return ok
	})

	assert.Equal(t, map[string]string{
		"EMPTY":       "",
		"REDIS_PORT":  "6380",
		"WITH_EQUALS": "a=b=c",
	}, env)
}

// matchTestFilePattern returns a matcher that applies path.Match to candidate
// file paths.
func matchTestFilePattern(pattern string) ConfigFilePathMatcher {
	return func(filePath VerifiedConfigFilePath) (bool, error) {
		return path.Match(pattern, filePath.String())
	}
}

// verifyTestConfigFilePath returns a verified path or fails the current test.
func verifyTestConfigFilePath(t testing.TB, value string) VerifiedConfigFilePath {
	t.Helper()
	verified, err := VerifyConfigFilePath(UnverifiedConfigFilePath(value))
	require.NoError(t, err)
	return verified
}

// verifyTestConfigFilePattern returns a verified pattern or fails the current test.
func verifyTestConfigFilePattern(t testing.TB, value string) VerifiedConfigFilePattern {
	t.Helper()
	verified, err := VerifyConfigFilePattern(UnverifiedConfigFilePattern(value))
	require.NoError(t, err)
	return verified
}

// verifyTestConfigFileSearch returns a scoped search or fails the current test.
func verifyTestConfigFileSearch(t testing.TB, root string, pattern string) ConfigFileSearch {
	t.Helper()
	search, err := NewConfigFileSearch(
		verifyTestConfigFilePath(t, root),
		verifyTestConfigFilePattern(t, pattern),
	)
	require.NoError(t, err)
	return search
}

// readConfigFileResults returns the successfully read files or fails the
// current test when a result contains a read error.
func readConfigFileResults(t testing.TB, results []ConfigFileReadResult) []ConfigFile {
	t.Helper()
	if results == nil {
		return nil
	}
	files := make([]ConfigFile, 0, len(results))
	for _, result := range results {
		file, err := result.Read()
		require.NoError(t, err)
		files = append(files, file)
	}
	return files
}

func TestVerifyConfigFileLocations(t *testing.T) {
	for _, value := range []string{"", "relative.conf", "/etc/redis/../outside.conf", "/etc/redis/control\n.conf"} {
		_, err := VerifyConfigFilePath(UnverifiedConfigFilePath(value))
		require.Error(t, err)
	}

	verifiedPath, err := VerifyConfigFilePath(UnverifiedConfigFilePath("/etc/redis/./redis.conf"))
	require.NoError(t, err)
	assert.Equal(t, "/etc/redis/redis.conf", verifiedPath.String())

	verifiedPattern, err := VerifyConfigFilePattern(UnverifiedConfigFilePattern("/etc/redis/./*.conf"))
	require.NoError(t, err)
	assert.Equal(t, "/etc/redis/*.conf", verifiedPattern.String())
}

func TestNewConfigFileSearch(t *testing.T) {
	root := verifyTestConfigFilePath(t, "/etc/redis")
	pattern := verifyTestConfigFilePattern(t, "/etc/redis/conf.d/*.conf")

	search, err := NewConfigFileSearch(root, pattern)

	require.NoError(t, err)
	assert.Equal(t, root, search.Root())
	assert.Equal(t, pattern, search.Pattern())
	assert.True(t, search.Contains(verifyTestConfigFilePath(t, "/etc/redis/conf.d/a.conf")))
	assert.False(t, search.Contains(verifyTestConfigFilePath(t, "/etc/redis-other/a.conf")))
}

func TestNewConfigFileSearchRejectsPatternOutsideRoot(t *testing.T) {
	_, err := NewConfigFileSearch(
		verifyTestConfigFilePath(t, "/etc/redis"),
		verifyTestConfigFilePattern(t, "/etc/redis-other/*.conf"),
	)

	require.Error(t, err)
}

func TestConfigFileSearchRoot(t *testing.T) {
	tests := []struct {
		root    string
		pattern string
		want    string
	}{
		{root: "/etc/redis", pattern: "/etc/redis/redis.conf", want: "/etc/redis/redis.conf"},
		{root: "/etc/redis", pattern: "/etc/redis/conf.d/*.conf", want: "/etc/redis/conf.d"},
		{root: "/etc/redis", pattern: "/etc/redis/conf.d/nested/*.conf", want: "/etc/redis/conf.d"},
		{root: "/etc/redis", pattern: "/etc/redis/file[0-9]?.conf", want: "/etc/redis"},
		{root: "/", pattern: "/etc/redis/*.conf", want: "/etc"},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			search := verifyTestConfigFileSearch(t, tt.root, tt.pattern)
			assert.Equal(t, tt.want, configFileSearchRoot(search).String())
		})
	}
}

func TestEscapeFindPathPattern(t *testing.T) {
	tests := []struct {
		name     string
		filePath string
		want     string
	}{
		{
			name:     "plain path",
			filePath: "/etc/redis/redis.conf",
			want:     "/etc/redis/redis.conf",
		},
		{
			name:     "pattern metacharacters",
			filePath: `/etc/redis/literal[*?\].conf`,
			want:     `/etc/redis/literal\[\*\?\\].conf`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, escapeFindPathPattern(verifyTestConfigFilePath(t, tt.filePath)))
		})
	}
}
