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

func TestBomCache_HitsWithinTTL(t *testing.T) {
	c := &bomCache{
		entries:    make(map[string]*bomCacheEntry),
		ttl:        time.Hour,
		maxEntries: 100,
	}

	// Seed cache manually
	c.entries["/var/db/receipts/com.example.bom"] = &bomCacheEntry{
		Lines:     []string{"./opt/example", "./opt/example/bin"},
		Timestamp: time.Now(),
	}

	result := c.getBomLines([]string{"/var/db/receipts/com.example.bom"})
	require.Len(t, result["/var/db/receipts/com.example.bom"], 2)
	assert.Equal(t, "./opt/example", result["/var/db/receipts/com.example.bom"][0])
}

func TestBomCache_ExpiredEntryRefetches(t *testing.T) {
	c := &bomCache{
		entries:    make(map[string]*bomCacheEntry),
		ttl:        5 * time.Millisecond,
		maxEntries: 100,
	}

	c.entries["/var/db/receipts/com.example.bom"] = &bomCacheEntry{
		Lines:     []string{"./stale"},
		Timestamp: time.Now().Add(-time.Second),
	}

	// After TTL, getBomLines should call batchLsbom for the expired key.
	// Since the BOM file doesn't exist, we'll get empty lines back.
	result := c.getBomLines([]string{"/var/db/receipts/com.example.bom"})
	lines := result["/var/db/receipts/com.example.bom"]
	assert.NotContains(t, lines, "./stale", "expired entry should not return stale data")
}

func TestBomCache_EvictsWhenFull(t *testing.T) {
	c := &bomCache{
		entries:    make(map[string]*bomCacheEntry),
		ttl:        time.Hour,
		maxEntries: 2,
	}

	oldest := time.Now().Add(-10 * time.Minute)
	c.entries["/bom/a"] = &bomCacheEntry{Lines: []string{}, Timestamp: oldest}
	c.entries["/bom/b"] = &bomCacheEntry{Lines: []string{}, Timestamp: time.Now()}

	// Inserting a third should evict the oldest (/bom/a)
	c.getBomLines([]string{"/bom/c"})

	assert.LessOrEqual(t, len(c.entries), 2)
	_, hasA := c.entries["/bom/a"]
	assert.False(t, hasA, "oldest entry should be evicted")
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
