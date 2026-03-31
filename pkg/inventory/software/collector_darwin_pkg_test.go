// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package software

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildPkgSummaryFromLines_AppOnly(t *testing.T) {
	lines := []string{
		"Applications/Google Chrome.app/Contents/Info.plist",
		"Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
	}

	summary := buildPkgSummaryFromLines(lines, "/")

	assert.True(t, summary.HasApplicationsApp)
	assert.False(t, summary.HasNonAppPayload)
	assert.Contains(t, summary.TopLevelPaths, "/Applications/Google Chrome.app")
	assert.True(t, shouldSkipPkgFromSummary(summary), "app-only packages should be skipped")
}

func TestBuildPkgSummaryFromLines_MixedPayloadKept(t *testing.T) {
	lines := []string{
		"Applications/Datadog Agent.app/Contents/Info.plist",
		"opt/datadog-agent/bin/agent",
		"opt/datadog-agent/etc/datadog.yaml",
	}

	summary := buildPkgSummaryFromLines(lines, "/")

	assert.True(t, summary.HasApplicationsApp)
	assert.True(t, summary.HasNonAppPayload)
	assert.Contains(t, summary.TopLevelPaths, "/Applications/Datadog Agent.app")
	assert.Contains(t, summary.TopLevelPaths, "/opt/datadog-agent")
	assert.True(t, shouldSkipPkgFromSummary(summary), "baseline semantics skip packages that include Applications app payload")
}

func TestBuildPkgSummaryFromLines_NonAppOnlyKept(t *testing.T) {
	lines := []string{
		"opt/example/bin/tool",
		"usr/local/bin/example",
	}

	summary := buildPkgSummaryFromLines(lines, "/")

	assert.False(t, summary.HasApplicationsApp)
	assert.True(t, summary.HasNonAppPayload)
	assert.Contains(t, summary.TopLevelPaths, "/opt/example")
	assert.Contains(t, summary.TopLevelPaths, "/usr/local/bin")
	assert.False(t, shouldSkipPkgFromSummary(summary), "non-app packages should be kept")
}

func TestBuildPkgSummaryFromLines_PrefixApplicationsApp(t *testing.T) {
	lines := []string{
		"Pages.app/Contents/Info.plist",
	}

	summary := buildPkgSummaryFromLines(lines, "Applications")

	assert.True(t, summary.HasApplicationsApp)
	assert.False(t, summary.HasNonAppPayload)
	assert.Contains(t, summary.TopLevelPaths, "/Applications/Pages.app")
	assert.True(t, shouldSkipPkgFromSummary(summary), "applications-prefix app-only package should be skipped")
}

func TestPkgFilesCacheGet_UsesCacheWithinTTL(t *testing.T) {
	cache := &pkgFilesCache{
		cache:      make(map[string]*pkgFilesCacheEntry),
		ttl:        time.Hour,
		maxEntries: 10,
	}

	var calls int
	cache.fetchSummaryFn = func(_ string, prefixPath string) pkgSummary {
		calls++
		return buildPkgSummaryFromLines([]string{"opt/example/bin/tool"}, prefixPath)
	}

	s1 := cache.get("com.example.pkg", "/")
	s2 := cache.get("com.example.pkg", "/")

	assert.Equal(t, 1, calls, "second call should hit cache")
	assert.Equal(t, s1, s2)
}

func TestPkgFilesCacheGet_RefetchesAfterTTL(t *testing.T) {
	cache := &pkgFilesCache{
		cache:      make(map[string]*pkgFilesCacheEntry),
		ttl:        5 * time.Millisecond,
		maxEntries: 10,
	}

	var calls int
	cache.fetchSummaryFn = func(_ string, prefixPath string) pkgSummary {
		calls++
		return buildPkgSummaryFromLines([]string{"opt/example/bin/tool"}, prefixPath)
	}

	_ = cache.get("com.example.pkg", "/")
	time.Sleep(20 * time.Millisecond)
	_ = cache.get("com.example.pkg", "/")

	assert.Equal(t, 2, calls, "expired cache entry should refetch")
}

func TestPkgFilesCacheGet_BoundedSizeEvictsOldest(t *testing.T) {
	cache := &pkgFilesCache{
		cache:      make(map[string]*pkgFilesCacheEntry),
		ttl:        time.Hour,
		maxEntries: 2,
	}
	cache.fetchSummaryFn = func(pkgID, prefixPath string) pkgSummary {
		return buildPkgSummaryFromLines([]string{"opt/" + pkgID + "/bin/tool"}, prefixPath)
	}

	_ = cache.get("pkg.one", "/")
	_ = cache.get("pkg.two", "/")
	_ = cache.get("pkg.three", "/")

	require.LessOrEqual(t, len(cache.cache), 2)
	_, hasOne := cache.cache[makePkgCacheKey("pkg.one", "/")]
	assert.False(t, hasOne, "oldest entry should be evicted once cache is full")
}

func TestFilterGenericSystemPaths(t *testing.T) {
	paths := []string{
		"/etc",
		"/var",
		"/tmp",
		"/System",
		"/opt/datadog-agent",
		"/usr/local/bin",
	}

	filtered := filterGenericSystemPaths(paths)

	assert.NotContains(t, filtered, "/etc")
	assert.NotContains(t, filtered, "/var")
	assert.NotContains(t, filtered, "/tmp")
	assert.NotContains(t, filtered, "/System")
	assert.Contains(t, filtered, "/opt/datadog-agent")
	assert.Contains(t, filtered, "/usr/local/bin")
}
