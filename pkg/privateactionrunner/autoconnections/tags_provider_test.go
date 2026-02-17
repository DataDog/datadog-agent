// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"context"
	"testing"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	pkgtaggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTagger is a simple test double for tagger.Component
type mockTagger struct {
	globalTags []string
	globalErr  error
}

func (m *mockTagger) Tag(entityID taggertypes.EntityID, cardinality taggertypes.TagCardinality) ([]string, error) {
	return nil, nil
}

func (m *mockTagger) GenerateContainerIDFromOriginInfo(originInfo origindetection.OriginInfo) (string, error) {
	return "", nil
}

func (m *mockTagger) Standard(entityID taggertypes.EntityID) ([]string, error) {
	return nil, nil
}

func (m *mockTagger) List() taggertypes.TaggerListResponse {
	return taggertypes.TaggerListResponse{}
}

func (m *mockTagger) GetEntity(entityID taggertypes.EntityID) (*taggertypes.Entity, error) {
	return nil, nil
}

func (m *mockTagger) Subscribe(subscriptionID string, filter *taggertypes.Filter) (taggertypes.Subscription, error) {
	return nil, nil
}

func (m *mockTagger) GetEntityHash(entityID taggertypes.EntityID, cardinality taggertypes.TagCardinality) string {
	return ""
}

func (m *mockTagger) AgentTags(cardinality taggertypes.TagCardinality) ([]string, error) {
	return nil, nil
}

func (m *mockTagger) GlobalTags(cardinality taggertypes.TagCardinality) ([]string, error) {
	return m.globalTags, m.globalErr
}

func (m *mockTagger) EnrichTags(tb tagset.TagsAccumulator, originInfo pkgtaggertypes.OriginInfo) {
}

// createMockTaggerWithClusterTags creates a mock tagger with cluster tags set
func createMockTaggerWithClusterTags(clusterName, clusterID string) *mockTagger {
	var tags []string
	if clusterName != "" {
		tags = append(tags, "kube_cluster_name:"+clusterName)
	}
	if clusterID != "" {
		tags = append(tags, "orch_cluster_id:"+clusterID)
	}

	return &mockTagger{
		globalTags: tags,
	}
}

func TestTagsProvider_WithClusterTags(t *testing.T) {
	mockTagger := createMockTaggerWithClusterTags("production", "abc-123-def")
	provider := NewTagsProvider(mockTagger)

	tags := provider.GetTags(context.Background(), "runner-abc", "host-01")

	require.Len(t, tags, 4)
	assert.Contains(t, tags, "runner-id:runner-abc")
	assert.Contains(t, tags, "hostname:host-01")
	assert.Contains(t, tags, "orch_cluster_id:abc-123-def")
	assert.Contains(t, tags, "kube_cluster_name:production")
}

func TestTagsProvider_WithEnvTag(t *testing.T) {
	mockTagger := &mockTagger{
		globalTags: []string{
			"env:production",
			"kube_cluster_name:my-cluster",
			"team:platform", // Not in tagSet, should be filtered out
		},
	}
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
	// Mock tagger with no cluster tags
	mockTagger := createMockTaggerWithClusterTags("", "")

	provider := NewTagsProvider(mockTagger)

	tags := provider.GetTags(context.Background(), "runner-abc", "host-01")

	require.Len(t, tags, 2)
	assert.Contains(t, tags, "runner-id:runner-abc")
	assert.Contains(t, tags, "hostname:host-01")
}

func TestTagsProvider_AllWhitelistedTags(t *testing.T) {
	mockTagger := &mockTagger{
		globalTags: []string{
			"env:staging",
			"cluster_name:my-cluster",
			"kube_cluster_name:k8s-cluster",
			"orch_cluster_id:cluster-123",
			"kube_distribution:eks",
			"region:us-east-1", // Not in tagSet, should be filtered
			"team:platform",    // Not in tagSet, should be filtered
		},
	}
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
