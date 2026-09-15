// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagfilter

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookupKeyLengthBounds(t *testing.T) {
	// "hostname" is protected, so maxKeyLen is at least 8 whatever is configured.
	f, _ := Compile(nil, []string{"eight888:*"})
	require.Equal(t, 8, f.maxKeyLen)

	assert.NotNil(t, f.lookupKey("eight888"), "a key of exactly maxKeyLen must resolve")
	assert.Nil(t, f.lookupKey("eight8888"), "a key longer than maxKeyLen must be rejected")
	assert.Nil(t, f.lookupKey(""), "an empty key must be rejected")
}

func TestLookupKeySameLengthSameFirstByte(t *testing.T) {
	// Both keys are length 4 starting with 't', so the first-byte check cannot
	// discriminate and equalFolded has to.
	f, report := Compile(nil, []string{"team:infra", "tier:cache"})
	require.Empty(t, report.Rejected)

	assert.False(t, f.Retains("team:infra"))
	assert.False(t, f.Retains("tier:cache"))
	assert.True(t, f.Retains("team:cache"), "values must not be swapped between keys")
	assert.True(t, f.Retains("tier:infra"))
	assert.True(t, f.Retains("tusk:infra"), "an unconfigured key of the same shape must miss")
}

func TestLookupKeySameLengthDifferentFirstByte(t *testing.T) {
	f, report := Compile(nil, []string{"team:*", "zone:*"})
	require.Empty(t, report.Rejected)

	assert.False(t, f.Retains("team:infra"))
	assert.False(t, f.Retains("zone:us-east-1a"))
	assert.True(t, f.Retains("mesh:linkerd"))
}

func TestLookupKeyPrefixIsNotAMatch(t *testing.T) {
	// "env" is protected, so both lengths have a populated bucket and neither
	// direction can match by landing in a non-empty one.
	f, report := Compile(nil, []string{"environment:*"})
	require.Empty(t, report.Rejected)

	assert.False(t, f.Retains("environment:prod"))
	assert.True(t, f.Retains("env:prod"), "a rule key must not match a shorter tag key")

	g, report := Compile(nil, []string{"env:*"})
	require.Empty(t, report.Rejected)
	assert.True(t, g.Retains("environment:prod"), "a rule key must not match a longer tag key")
}

// TestLongBucketHoldsMixedLengths pins that the overflow bucket compares
// lengths. Length buckets are homogeneous, long is not, so a shorter tag key
// must not match a longer rule key that shares its bytes.
func TestLongBucketHoldsMixedLengths(t *testing.T) {
	short := strings.Repeat("0", maxBucketedKeyLen+1)
	long := strings.Repeat("0", maxBucketedKeyLen+2)
	f, report := Compile(nil, []string{short + ":*", long + ":*"})
	require.Empty(t, report.Rejected)
	require.Len(t, f.long, 2)

	assert.False(t, f.Retains(short+":x"))
	assert.False(t, f.Retains(long+":x"))
	// Between the two configured lengths there is nothing, and beyond them the
	// maxKeyLen guard applies; neither may resolve to the other's rules.
	assert.True(t, f.Retains(strings.Repeat("0", maxBucketedKeyLen)+":x"))
	assert.True(t, f.Retains(long+"0:x"))
}

// TestPathologicalRuleKeyDoesNotSizeAllocation pins §4.3: bucket storage is
// capped by maxBucketedKeyLen, so a configured key of arbitrary length cannot
// size an allocation that is made once per source.
func TestPathologicalRuleKeyDoesNotSizeAllocation(t *testing.T) {
	huge := strings.Repeat("k", 1<<20)
	f, report := Compile(nil, []string{huge + ":*"})
	require.Empty(t, report.Rejected)

	assert.LessOrEqual(t, len(f.byLen), maxBucketedKeyLen+1)
	assert.Len(t, f.long, 1)
	assert.Equal(t, len(huge), f.maxKeyLen)

	assert.False(t, f.Retains(huge+":anything"))
	assert.True(t, f.Retains("team:infra"), "ordinary keys still resolve")
}

// TestLongBucketUnreachableWithoutLongKeys pins the composition of the two
// bounds noted in §4.3: with no over-length rule key, the maxKeyLen guard
// already rejects everything that would have reached long.
func TestLongBucketUnreachableWithoutLongKeys(t *testing.T) {
	f, _ := Compile(nil, []string{"team:*", "kube_replica_set:*"})
	require.Nil(t, f.long)
	assert.Less(t, f.maxKeyLen, len(f.byLen))
}

func TestEqualFolded(t *testing.T) {
	tests := []struct {
		folded, s string
		want      bool
	}{
		{"team", "team", true},
		{"team", "TEAM", true},
		{"team", "tEaM", true},
		{"team", "tier", false},
		{"team", "teaM", true},
		{"t", "T", true},
		{"team", "teab", false},
	}
	for _, tt := range tests {
		t.Run(tt.folded+"/"+tt.s, func(t *testing.T) {
			assert.Equal(t, tt.want, equalFolded(tt.folded, tt.s))
		})
	}
}

// manyKeysFilter excludes count distinct keys, all of the same length and all
// sharing a first byte, which is the worst case for the bucket scan.
func manyKeysFilter(count int) *Filters {
	patterns := make([]string, 0, count)
	for i := range count {
		patterns = append(patterns, fmt.Sprintf("team_%c%c:*", 'a'+i/26, 'a'+i%26))
	}
	f, _ := Compile(nil, patterns)
	return f
}

// largeRuleSetFilter excludes 200 distinct keys of varied length and first byte.
func largeRuleSetFilter() *Filters {
	patterns := make([]string, 0, 200)
	for i := range 200 {
		patterns = append(patterns, fmt.Sprintf("%ckey_%03d:*", 'a'+i%26, i))
	}
	f, _ := Compile(nil, patterns)
	return f
}

func TestWorstCaseBucketFiltersAreEngaged(t *testing.T) {
	many := manyKeysFilter(32)
	require.False(t, many.Retains("team_aa:x"))
	require.False(t, many.Retains("team_bf:x"))
	require.True(t, many.Retains("team_zz:x"), "only the first 32 keys are configured")

	large := largeRuleSetFilter()
	require.False(t, large.Retains("akey_000:x"))
	require.False(t, large.Retains("rkey_199:x"))
	require.True(t, large.Retains("akey_999:x"))
}

// BenchmarkLookupManySameShape is the pathological scan: 32 rule keys of equal
// length sharing a first byte, so every candidate reaches equalFolded.
func BenchmarkLookupManySameShape(b *testing.B) {
	f := manyKeysFilter(32)
	tags := realisticK8sTags()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = f.Keep(tags)
	}
}

// BenchmarkLookupLargeRuleSet proves there is no cliff at a rule-set size far
// beyond any realistic config (§4.5).
func BenchmarkLookupLargeRuleSet(b *testing.B) {
	f := largeRuleSetFilter()
	tags := realisticK8sTags()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = f.Keep(tags)
	}
}
