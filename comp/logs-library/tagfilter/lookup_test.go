// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagfilter

import (
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

func TestLookupKeyCollision(t *testing.T) {
	// Both keys have the same length and first byte, so lookup must compare the
	// complete key rather than allowing rules to bleed between buckets.
	f, report := Compile(nil, []string{"team:infra", "tier:cache"})
	require.Empty(t, report.Rejected)

	assert.False(t, f.Retains("team:infra"))
	assert.False(t, f.Retains("tier:cache"))
	assert.True(t, f.Retains("team:cache"))
	assert.True(t, f.Retains("tusk:infra"))
}

// TestLongBucketHoldsMixedLengths pins that the overflow bucket compares
// lengths. Length buckets are homogeneous, but the overflow bucket is not.
func TestLongBucketHoldsMixedLengths(t *testing.T) {
	short := strings.Repeat("0", maxBucketedKeyLen+1)
	long := strings.Repeat("0", maxBucketedKeyLen+2)
	f, report := Compile(nil, []string{short + ":*", long + ":*"})
	require.Empty(t, report.Rejected)
	require.Len(t, f.long, 2)

	assert.False(t, f.Retains(short+":x"))
	assert.False(t, f.Retains(long+":x"))
	assert.True(t, f.Retains(strings.Repeat("0", maxBucketedKeyLen)+":x"))
	assert.True(t, f.Retains(long+"0:x"))
}

// TestPathologicalRuleKeyDoesNotSizeAllocation ensures an arbitrarily long
// configured key cannot size the per-source bucket allocation.
func TestPathologicalRuleKeyDoesNotSizeAllocation(t *testing.T) {
	huge := strings.Repeat("k", 1<<20)
	f, report := Compile(nil, []string{huge + ":*"})
	require.Empty(t, report.Rejected)

	assert.LessOrEqual(t, len(f.byLen), maxBucketedKeyLen+1)
	assert.Len(t, f.long, 1)
	assert.Equal(t, len(huge), f.maxKeyLen)
	assert.False(t, f.Retains(huge+":anything"))
}
