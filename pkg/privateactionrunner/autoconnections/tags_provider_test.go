// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"context"
	"testing"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTagsProvider_WithClusterTags(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockTagger.SetGlobalTags([]string{"kube_cluster_name:production", "orch_cluster_id:abc-123-def"}, nil, nil, nil)
	provider := NewTagsProvider(mockTagger)

	tags := provider.GetTags(context.Background(), "runner-abc", "host-01")

	require.Len(t, tags, 4)
	assert.Contains(t, tags, "runner-id:runner-abc")
	assert.Contains(t, tags, "hostname:host-01")
	assert.Contains(t, tags, "orch_cluster_id:abc-123-def")
	assert.Contains(t, tags, "kube_cluster_name:production")
}

func TestTagsProvider_WithEnvTag(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockTagger.SetGlobalTags([]string{
		"env:production",
		"kube_cluster_name:my-cluster",
		"team:platform", // Not in tagSet, should be filtered out
	}, nil, nil, nil)
	provider := NewTagsProvider(mockTagger)

	tags := provider.GetTags(context.Background(), "runner-xyz", "host-02")

	require.Len(t, tags, 4)
	assert.Contains(t, tags, "runner-id:runner-xyz")
	assert.Contains(t, tags, "hostname:host-02")
	assert.Contains(t, tags, "env:production")
	assert.Contains(t, tags, "kube_cluster_name:my-cluster")
	assert.NotContains(t, tags, "team:platform") // Filtered out
}

func TestTagsProvider_NoClusterTagsAvailable(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	provider := NewTagsProvider(mockTagger)

	tags := provider.GetTags(context.Background(), "runner-abc", "host-01")

	require.Len(t, tags, 2)
	assert.Contains(t, tags, "runner-id:runner-abc")
	assert.Contains(t, tags, "hostname:host-01")
}

func TestTagsProvider_AllWhitelistedTags(t *testing.T) {
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mockTagger.SetGlobalTags([]string{
		"env:staging",
		"cluster_name:my-cluster",
		"kube_cluster_name:k8s-cluster",
		"orch_cluster_id:cluster-123",
		"kube_distribution:eks",
		"region:us-east-1", // Not in tagSet, should be filtered
		"team:platform",    // Not in tagSet, should be filtered
	}, nil, nil, nil)
	provider := NewTagsProvider(mockTagger)

	tags := provider.GetTags(context.Background(), "runner-abc", "host-01")

	// Should have: runner-id, hostname + 5 whitelisted tags
	require.Len(t, tags, 7)
	assert.Contains(t, tags, "runner-id:runner-abc")
	assert.Contains(t, tags, "hostname:host-01")
	assert.Contains(t, tags, "env:staging")
	assert.Contains(t, tags, "cluster_name:my-cluster")
	assert.Contains(t, tags, "kube_cluster_name:k8s-cluster")
	assert.Contains(t, tags, "orch_cluster_id:cluster-123")
	assert.Contains(t, tags, "kube_distribution:eks")

	// Filtered out
	assert.NotContains(t, tags, "region:us-east-1")
	assert.NotContains(t, tags, "team:platform")
}
