// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package noisyneighbor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWatchlistChanged(t *testing.T) {
	check := &NoisyNeighborCheck{}

	// nil → empty: no change
	assert.False(t, check.watchlistChanged(nil), "nil → nil should not be a change")

	// nil → non-empty: change
	assert.True(t, check.watchlistChanged([]uint64{1, 2, 3}), "nil → {1,2,3} should be a change")

	// same set: no change
	assert.False(t, check.watchlistChanged([]uint64{1, 2, 3}), "{1,2,3} → {1,2,3} should not be a change")

	// same set, different order: no change
	assert.False(t, check.watchlistChanged([]uint64{3, 1, 2}), "{1,2,3} → {3,1,2} should not be a change")

	// subset: change
	assert.True(t, check.watchlistChanged([]uint64{1, 2}), "{1,2,3} → {1,2} should be a change")

	// superset: change
	assert.True(t, check.watchlistChanged([]uint64{1, 2, 4}), "{1,2} → {1,2,4} should be a change")

	// disjoint: change
	assert.True(t, check.watchlistChanged([]uint64{5, 6}), "{1,2,4} → {5,6} should be a change")

	// non-empty → empty: change
	assert.True(t, check.watchlistChanged(nil), "{5,6} → nil should be a change")

	// empty → empty: no change
	assert.False(t, check.watchlistChanged(nil), "nil → nil should not be a change")
}

func TestParseConfigDefaults(t *testing.T) {
	c := &NoisyNeighborConfig{}
	err := c.Parse([]byte("{}"))
	assert.NoError(t, err)
	assert.Equal(t, defaultPSIThreshold, c.PSIThreshold)
	assert.Equal(t, defaultThrottleRatio, c.ThrottleRatio)
	assert.Equal(t, defaultStealThreshold, c.StealThreshold)
	assert.Equal(t, defaultMaxWatchlistSize, c.MaxWatchlistSize)
	assert.Equal(t, defaultMaxTopNPreemptors, c.MaxTopNPreemptors)
	assert.Equal(t, defaultMaxNonContainerCgroups, c.MaxNonContainerCgroups)
	assert.Equal(t, uint64(defaultMinForeignPreemptionsImpact), c.MinForeignPreemptionsImpact)
}

func TestParseConfigClampsWatchlistSize(t *testing.T) {
	c := &NoisyNeighborConfig{}
	err := c.Parse([]byte("max_watchlist_size: 500"))
	assert.NoError(t, err)
	assert.Equal(t, hardMaxWatchlistSize, c.MaxWatchlistSize)
}

// TestPreemptorIdentityTags pins the tag-extraction order. Indexing the
// tagger's slice positionally (the previous behavior) produced unstable
// values like "image_tag:1.2.3" or "k8s_cluster:foo" depending on what the
// tagger emitted first.
func TestPreemptorIdentityTags(t *testing.T) {
	tests := []struct {
		name        string
		preemptor   []string
		cgroupID    uint64
		wantPrimary string
		wantNS      string
	}{
		{
			name: "container_name preferred when present",
			preemptor: []string{
				"image_tag:1.2.3",
				"container_name:nginx",
				"pod_name:web-abc",
				"kube_namespace:default",
				"kube_deployment:web",
			},
			wantPrimary: "preemptor:nginx",
			wantNS:      "preemptor_kube_namespace:default",
		},
		{
			name: "pod_name fallback when no container_name",
			preemptor: []string{
				"image_tag:1.2.3",
				"pod_name:web-abc",
				"kube_namespace:default",
				"kube_deployment:web",
			},
			wantPrimary: "preemptor:web-abc",
			wantNS:      "preemptor_kube_namespace:default",
		},
		{
			name: "deployment fallback when no container/pod",
			preemptor: []string{
				"kube_deployment:web",
				"kube_namespace:default",
			},
			wantPrimary: "preemptor:web",
			wantNS:      "preemptor_kube_namespace:default",
		},
		{
			name: "statefulset fallback",
			preemptor: []string{
				"kube_statefulset:redis",
				"kube_namespace:cache",
			},
			wantPrimary: "preemptor:redis",
			wantNS:      "preemptor_kube_namespace:cache",
		},
		{
			name:        "cgroup-id fallback when nothing matches",
			preemptor:   []string{"image_tag:1.2.3", "k8s_cluster:foo"},
			cgroupID:    12345,
			wantPrimary: "preemptor:cgroup-12345",
		},
		{
			name:        "empty tag list yields cgroup-id fallback",
			preemptor:   nil,
			cgroupID:    99,
			wantPrimary: "preemptor:cgroup-99",
		},
		{
			name: "tag order does not affect result (stable across permutations)",
			preemptor: []string{
				"k8s_cluster:foo",
				"kube_namespace:default",
				"container_name:nginx",
				"image_name:nginx",
			},
			wantPrimary: "preemptor:nginx",
			wantNS:      "preemptor_kube_namespace:default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := preemptorIdentityTags(tt.preemptor, tt.cgroupID)
			assert.Contains(t, got, tt.wantPrimary)
			if tt.wantNS != "" {
				assert.Contains(t, got, tt.wantNS)
			}
		})
	}
}

// TestPreemptorIdentityTagsDoesNotPickPositionFirst is the regression test
// for the original bug: the previous implementation used preemptorTags[0]
// directly, so the value depended on the tagger's slice ordering.
func TestPreemptorIdentityTagsDoesNotPickPositionFirst(t *testing.T) {
	// All permutations of these tags should produce the same primary tag.
	// In the buggy implementation, each permutation would yield a different
	// preemptor: value (image_tag:..., k8s_cluster:..., kube_namespace:...).
	tags := []string{
		"image_tag:1.2.3",
		"k8s_cluster:foo",
		"kube_namespace:default",
		"container_name:nginx",
	}

	first := preemptorIdentityTags(tags, 0)
	for i := 0; i < len(tags); i++ {
		// rotate
		rotated := append([]string{}, tags[i:]...)
		rotated = append(rotated, tags[:i]...)
		got := preemptorIdentityTags(rotated, 0)
		assert.Equal(t, first, got, "tag order should not affect primary preemptor: tag")
	}
}
