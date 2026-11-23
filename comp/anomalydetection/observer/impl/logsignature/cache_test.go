// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package logsignature

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/tokenizer"
)

func cacheTestConfig() CacheConfig {
	return CacheConfig{MaxEvalBytes: 512, MaxPatterns: 8, MaxPatternsPerGroup: 4, MaxGroups: 4, IdleTTLSeconds: 10, MatchThreshold: 0.9}
}

func TestCacheSharedMatchingAndOwnedTokens(t *testing.T) {
	for _, threshold := range []float64{0.5, 0.9, 1} {
		cfg := cacheTestConfig()
		cfg.MatchThreshold = threshold
		c := NewCache(cfg)
		a, _ := c.Match(Group{}, "ERROR request failed from 10.0.0.1", 1)
		retained := slices.Clone(a.tokens)
		b, _ := c.Match(Group{}, "ERROR request timeout from 192.168.100.200", 2)
		tok := tokenizer.NewTokenizer(cfg.MaxEvalBytes)
		incoming, _ := tok.Tokenize([]byte("ERROR request timeout from 192.168.100.200"))
		require.Equal(t, tokenizer.IsMatch(retained, incoming, threshold), a == b)
		require.Equal(t, retained, a.tokens, "a retained representative must not alias tokenizer scratch")
	}
	// Shared matching compares the shorter prefix, including at threshold 1.
	c := NewCache(cacheTestConfig())
	c.config.MatchThreshold = 1
	a, _ := c.Match(Group{}, "abc", 1)
	b, _ := c.Match(Group{}, "abc 123", 2)
	require.Same(t, a, b)
}

func TestCacheFrequencyOrderedFirstMatch(t *testing.T) {
	cfg := cacheTestConfig()
	cfg.MatchThreshold = 1
	c := NewCache(cfg)
	a, _ := c.Match(Group{}, "a 123", 1)
	b, _ := c.Match(Group{}, "abc 123", 2)
	c.Match(Group{}, "abc 123", 3)
	require.NotSame(t, a, b)
	c.config.MatchThreshold = 0.5
	got, _ := c.Match(Group{}, "a 123", 4)
	require.Same(t, b, got, "most frequent matching representative wins, even when another matches exactly")
}

func TestCacheBoundsAndCleanup(t *testing.T) {
	for _, test := range []struct {
		name           string
		edit           func(*CacheConfig)
		groupA, groupB Group
	}{
		{"global", func(c *CacheConfig) { c.MaxPatterns = 1 }, Group{}, Group{}},
		{"per group", func(c *CacheConfig) { c.MaxPatternsPerGroup = 1 }, Group{}, Group{}},
		{"groups", func(c *CacheConfig) { c.MaxGroups = 1 }, Group{Service: "a"}, Group{Service: "b"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg := cacheTestConfig()
			test.edit(&cfg)
			c := NewCache(cfg)
			a, _ := c.Match(test.groupA, "abc", 1)
			b, evicted := c.Match(test.groupB, "123456", 2)
			require.Equal(t, []string{a.SeriesName}, evicted)
			require.NotEqual(t, a.SeriesName, b.SeriesName)
			require.Equal(t, 1, c.Len())
			require.Len(t, c.groups, 1)
			again, _ := c.Match(test.groupA, "abc", 3)
			require.NotEqual(t, a.SeriesName, again.SeriesName, "evicted identities must not be reused")
		})
	}
}

func TestCacheExpiryLateLogsAndEmptyAdmission(t *testing.T) {
	c := NewCache(cacheTestConfig())
	a, _ := c.Match(Group{Service: "a"}, "abc", 100)
	_, evicted := c.Match(Group{Service: "new"}, "", 105)
	require.Empty(t, evicted)
	require.Len(t, c.groups, 1, "empty input must not admit a group")
	c.Match(Group{Service: "a"}, "abc", 90)
	_, evicted = c.Match(Group{}, "", 116)
	require.Equal(t, []string{a.SeriesName}, evicted)
	require.Zero(t, c.Len())
	require.Empty(t, c.groups)
	require.Zero(t, c.groupLRU.Len())
	c.Match(Group{}, "abc", 117)
	c.Reset()
	require.Zero(t, c.Len())
	require.Empty(t, c.groups)
	require.Zero(t, c.watermark)
}

func TestCacheGroupIsolationAndHotPath(t *testing.T) {
	c := NewCache(cacheTestConfig())
	a, _ := c.Match(Group{Host: "a"}, "abc 123", 1)
	b, _ := c.Match(Group{Host: "b"}, "abc 123", 1)
	require.NotEqual(t, a.SeriesName, b.SeriesName)
	allocs := testing.AllocsPerRun(1000, func() {
		got, evicted := c.Match(Group{Host: "a"}, "abc 123", 2)
		if got != a || len(evicted) != 0 {
			t.Fatal("warm lookup changed identity")
		}
	})
	require.Zero(t, allocs, "warm lookup must not allocate tokens or signature strings")
}

func TestCacheInputWindowAndChurn(t *testing.T) {
	cfg := cacheTestConfig()
	cfg.MaxEvalBytes = 16
	c := NewCache(cfg)
	prefix := strings.Repeat("a", 16)
	a, _ := c.Match(Group{}, prefix+" 123", 1)
	b, _ := c.Match(Group{}, prefix+" xyz", 1)
	require.Same(t, a, b)
	for i := 0; i < 1000; i++ {
		c.Match(Group{Host: strings.Repeat("x", i%20)}, strings.Repeat("a", i%10+1), int64(i+2))
		require.LessOrEqual(t, c.Len(), cfg.MaxPatterns)
		require.LessOrEqual(t, len(c.groups), cfg.MaxGroups)
		for _, g := range c.groups {
			require.Equal(t, len(g.patterns), g.lru.Len())
			require.LessOrEqual(t, len(g.patterns), cfg.MaxPatternsPerGroup)
			for index, p := range g.patterns {
				require.Equal(t, index, p.index)
			}
		}
	}
}
