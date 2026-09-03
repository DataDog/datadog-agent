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

// verifiedConfigFilePathStrings returns the string representation of paths.
func verifiedConfigFilePathStrings(paths []VerifiedConfigFilePath) []string {
	if paths == nil {
		return nil
	}
	values := make([]string, 0, len(paths))
	for _, path := range paths {
		values = append(values, path.String())
	}
	return values
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

func TestFilePatternSearchRoot(t *testing.T) {
	assert.Equal(t, "/etc/redis/redis.conf", filePatternSearchRoot(verifyTestConfigFilePattern(t, "/etc/redis/redis.conf")).String())
	assert.Equal(t, "/etc/redis/conf.d", filePatternSearchRoot(verifyTestConfigFilePattern(t, "/etc/redis/conf.d/*.conf")).String())
	assert.Equal(t, "/etc/redis", filePatternSearchRoot(verifyTestConfigFilePattern(t, "/etc/redis/file[0-9]?.conf")).String())
}
