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
		"./Applications/Google Chrome.app",
		"./Applications/Google Chrome.app/Contents",
		"./Applications/Google Chrome.app/Contents/MacOS",
	}

	summary := buildPkgSummaryFromLines(lines, "/")

	assert.True(t, summary.HasApplicationsApp)
	assert.False(t, summary.HasNonAppPayload)
	assert.Contains(t, summary.TopLevelPaths, "/Applications/Google Chrome.app")
	assert.True(t, shouldSkipPkgFromSummary(summary), "app-only packages should be skipped")
}

func TestBuildPkgSummaryFromLines_MixedPayloadSkipped(t *testing.T) {
	lines := []string{
		"./Applications/Datadog Agent.app",
		"./Applications/Datadog Agent.app/Contents",
		"./opt/datadog-agent",
		"./opt/datadog-agent/bin",
		"./opt/datadog-agent/etc",
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
		"./opt/example",
		"./opt/example/bin",
		"./usr/local/bin",
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
		"./Pages.app",
		"./Pages.app/Contents",
	}

	summary := buildPkgSummaryFromLines(lines, "Applications")

	assert.True(t, summary.HasApplicationsApp)
	assert.False(t, summary.HasNonAppPayload)
	assert.Contains(t, summary.TopLevelPaths, "/Applications/Pages.app")
	assert.True(t, shouldSkipPkgFromSummary(summary), "applications-prefix app-only package should be skipped")
}

func TestBuildPkgSummaryFromDigest_WithRelativeTopLevelTokens(t *testing.T) {
	digest := bomDigest{
		HasApplicationsApp: true,
		HasNonAppPayload:   true,
		TopLevelTokens: []topLevelToken{
			{Value: "/Applications/Datadog Agent.app"},
			{Value: "Datadog Agent", Relative: true},
		},
	}

	summary := buildPkgSummaryFromDigest(digest, "/opt")

	assert.True(t, summary.HasApplicationsApp)
	assert.True(t, summary.HasNonAppPayload)
	assert.Contains(t, summary.TopLevelPaths, "/Applications/Datadog Agent.app")
	assert.Contains(t, summary.TopLevelPaths, "/opt/Datadog Agent")
}

func TestBuildBomDigest_DedupesTopLevelsAndAppPaths(t *testing.T) {
	lines := []string{
		"./Applications/Pages.app",
		"./Applications/Pages.app",
		"./Applications/Pages.app/Contents",
		"./foo",
		"./foo/bar",
	}

	digest := buildBomDigest(lines)

	assert.True(t, digest.HasApplicationsApp)
	assert.True(t, digest.HasNonAppPayload)
	assert.Equal(t, []string{"Applications/Pages.app"}, digest.AppPaths)

	// Ensure top-level tokens were deduplicated.
	seen := make(map[topLevelToken]struct{})
	for _, tok := range digest.TopLevelTokens {
		seen[tok] = struct{}{}
	}
	assert.Len(t, digest.TopLevelTokens, len(seen))
}

func TestBomDigestBuilder_AddLineNormalizesAndDedupes(t *testing.T) {
	builder := newBomDigestBuilder()

	builder.addLine("  ./Applications/Pages.app  ")
	builder.addLine("./Applications/Pages.app")
	builder.addLine("./Applications/Pages.app/Contents")
	builder.addLine("./opt/example")

	digest := builder.result()

	assert.True(t, digest.HasApplicationsApp)
	assert.True(t, digest.HasNonAppPayload)
	assert.Equal(t, []string{"Applications/Pages.app"}, digest.AppPaths)
	assert.Equal(t, []topLevelToken{
		{Value: "/Applications/Pages.app"},
		{Value: "/opt/example"},
	}, digest.TopLevelTokens)
}

func TestNormalizeBomLine(t *testing.T) {
	assert.Equal(t, "", normalizeBomLine(""))
	assert.Equal(t, "", normalizeBomLine("."))
	assert.Equal(t, "", normalizeBomLine("./"))
	assert.Equal(t, "foo", normalizeBomLine("  ./foo  "))
	assert.Equal(t, "Applications/App.app", normalizeBomLine("./Applications/App.app"))
}

func TestTopLevelTokenFromLine(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		expected topLevelToken
		ok       bool
	}{
		{name: "usr", line: "usr/local/bin", expected: topLevelToken{Value: "/usr/local/bin"}, ok: true},
		{name: "library", line: "Library/Application Support", expected: topLevelToken{Value: "/Library/Application Support"}, ok: true},
		{name: "opt", line: "opt/datadog-agent/bin", expected: topLevelToken{Value: "/opt/datadog-agent"}, ok: true},
		{name: "applications", line: "Applications/Foo.app/Contents", expected: topLevelToken{Value: "/Applications/Foo.app"}, ok: true},
		{name: "system", line: "System/Library/Extensions", expected: topLevelToken{Value: "/System/Library/Extensions"}, ok: true},
		{name: "default", line: "Datadog Agent/bin", expected: topLevelToken{Value: "Datadog Agent", Relative: true}, ok: true},
		{name: "empty", line: "", expected: topLevelToken{}, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, ok := topLevelTokenFromLine(tt.line)
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.expected, token)
		})
	}
}

func TestBomCache_HitsWithinTTL(t *testing.T) {
	originalCacheToggle := enableBomDigestCache
	enableBomDigestCache = true
	t.Cleanup(func() {
		enableBomDigestCache = originalCacheToggle
	})

	c := &bomCache{
		entries:    make(map[string]*bomCacheEntry),
		ttl:        time.Hour,
		maxEntries: 100,
	}

	// Seed cache manually
	c.entries["/var/db/receipts/com.example.bom"] = &bomCacheEntry{
		Digest: bomDigest{
			TopLevelTokens: []topLevelToken{
				{Value: "/opt/example"},
			},
		},
		Timestamp: time.Now(),
	}

	result := c.getBomDigests([]string{"/var/db/receipts/com.example.bom"})
	require.Len(t, result["/var/db/receipts/com.example.bom"].TopLevelTokens, 1)
	assert.Equal(t, "/opt/example", result["/var/db/receipts/com.example.bom"].TopLevelTokens[0].Value)
}

func TestBomCache_ExpiredEntryRefetches(t *testing.T) {
	originalCacheToggle := enableBomDigestCache
	enableBomDigestCache = true
	t.Cleanup(func() {
		enableBomDigestCache = originalCacheToggle
	})

	originalFetcher := batchLsbomFetcher
	t.Cleanup(func() {
		batchLsbomFetcher = originalFetcher
	})

	batchLsbomFetcher = func(bomPaths []string) map[string]bomFetchOutcome {
		require.Equal(t, []string{"/var/db/receipts/com.example.bom"}, bomPaths)
		return map[string]bomFetchOutcome{
			"/var/db/receipts/com.example.bom": {
				Digest: bomDigest{
					TopLevelTokens: []topLevelToken{{Value: "/opt/example"}},
				},
				Cacheable: true,
			},
		}
	}

	c := &bomCache{
		entries:    make(map[string]*bomCacheEntry),
		ttl:        5 * time.Millisecond,
		maxEntries: 100,
	}

	c.entries["/var/db/receipts/com.example.bom"] = &bomCacheEntry{
		Digest: bomDigest{
			TopLevelTokens: []topLevelToken{
				{Value: "/stale"},
			},
		},
		Timestamp: time.Now().Add(-time.Second),
	}

	// After TTL, getBomDigests should call batchLsbom for the expired key.
	// Since the BOM file doesn't exist, we'll get empty lines back.
	result := c.getBomDigests([]string{"/var/db/receipts/com.example.bom"})
	digest := result["/var/db/receipts/com.example.bom"]
	assert.Equal(t, []topLevelToken{{Value: "/opt/example"}}, digest.TopLevelTokens)
}

func TestBomCache_EvictsWhenFull(t *testing.T) {
	originalCacheToggle := enableBomDigestCache
	enableBomDigestCache = true
	t.Cleanup(func() {
		enableBomDigestCache = originalCacheToggle
	})

	originalFetcher := batchLsbomFetcher
	t.Cleanup(func() {
		batchLsbomFetcher = originalFetcher
	})

	batchLsbomFetcher = func(bomPaths []string) map[string]bomFetchOutcome {
		require.Equal(t, []string{"/bom/c"}, bomPaths)
		return map[string]bomFetchOutcome{
			"/bom/c": {
				Digest:    bomDigest{},
				Cacheable: true,
			},
		}
	}

	c := &bomCache{
		entries:    make(map[string]*bomCacheEntry),
		ttl:        time.Hour,
		maxEntries: 2,
	}

	oldest := time.Now().Add(-10 * time.Minute)
	c.entries["/bom/a"] = &bomCacheEntry{Digest: bomDigest{}, Timestamp: oldest}
	c.entries["/bom/b"] = &bomCacheEntry{Digest: bomDigest{}, Timestamp: time.Now()}

	// Inserting a third should evict the oldest (/bom/a)
	c.getBomDigests([]string{"/bom/c"})

	assert.LessOrEqual(t, len(c.entries), 2)
	_, hasA := c.entries["/bom/a"]
	assert.False(t, hasA, "oldest entry should be evicted")
}

func TestBomCache_DoesNotCacheNonCacheableFetches(t *testing.T) {
	originalCacheToggle := enableBomDigestCache
	enableBomDigestCache = true
	t.Cleanup(func() {
		enableBomDigestCache = originalCacheToggle
	})

	originalFetcher := batchLsbomFetcher
	t.Cleanup(func() {
		batchLsbomFetcher = originalFetcher
	})

	batchLsbomFetcher = func(bomPaths []string) map[string]bomFetchOutcome {
		require.Equal(t, []string{"/bom/fail"}, bomPaths)
		return map[string]bomFetchOutcome{
			"/bom/fail": {
				Digest:    bomDigest{TopLevelTokens: []topLevelToken{{Value: "/transient"}}},
				Cacheable: false,
			},
		}
	}

	c := &bomCache{
		entries:    make(map[string]*bomCacheEntry),
		ttl:        time.Hour,
		maxEntries: 2,
	}

	result := c.getBomDigests([]string{"/bom/fail"})
	assert.Equal(t, []topLevelToken{{Value: "/transient"}}, result["/bom/fail"].TopLevelTokens)
	_, exists := c.entries["/bom/fail"]
	assert.False(t, exists, "non-cacheable fetch results should not be stored")
}

func TestBatchLsbom_EmptyInput(t *testing.T) {
	result := batchLsbom(nil)
	assert.Nil(t, result)

	result = batchLsbom([]string{})
	assert.Nil(t, result)
}

func TestBuildEntryFromReceipt_SkipsAppPackage(t *testing.T) {
	receipt := pkgReceiptInfo{
		packageID:   "com.google.Chrome",
		version:     "1.0",
		installDate: "2026-01-01",
		prefixPath:  "/",
		bomPath:     "/var/db/receipts/com.google.Chrome.bom",
	}
	summary := pkgSummary{
		HasApplicationsApp: true,
		HasNonAppPayload:   false,
		TopLevelPaths:      []string{"/Applications/Google Chrome.app"},
	}

	entry := buildEntryFromReceipt(receipt, summary, true)
	assert.Nil(t, entry, "app packages should be skipped")
}

func TestBuildEntryFromReceipt_KeepsNonAppPackage(t *testing.T) {
	receipt := pkgReceiptInfo{
		packageID:   "com.example.tool",
		version:     "2.0",
		installDate: "2026-01-01",
		prefixPath:  "/",
		bomPath:     "/var/db/receipts/com.example.tool.bom",
	}
	summary := pkgSummary{
		HasApplicationsApp: false,
		HasNonAppPayload:   true,
		TopLevelPaths:      []string{"/opt/example"},
	}

	entry := buildEntryFromReceipt(receipt, summary, true)
	assert.NotNil(t, entry)
	assert.Equal(t, "com.example.tool", entry.DisplayName)
	assert.Equal(t, "2.0", entry.Version)
	assert.Equal(t, softwareTypePkg, entry.Source)
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
