// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
)

// MockTagger implements Tagger for testing
type MockTagger struct {
	// ContainerTags maps container ID to tags
	ContainerTags map[string][]string
	// SandboxTags maps sandbox container ID to tags
	SandboxTags map[string][]string
	// GlobalTagsValue holds the global tags
	GlobalTagsValue []string
	// Error to return on Tag calls
	TagError error
}

func (m *MockTagger) Tag(entity types.EntityID, _ types.TagCardinality) ([]string, error) {
	if m.TagError != nil {
		return nil, m.TagError
	}

	prefix := entity.GetPrefix()
	id := entity.GetID()

	switch prefix {
	case types.ContainerID:
		if tags, ok := m.ContainerTags[id]; ok {
			return tags, nil
		}
	case types.KubernetesPodSandbox:
		if tags, ok := m.SandboxTags[id]; ok {
			return tags, nil
		}
	}

	return nil, nil
}

func (m *MockTagger) GlobalTags(_ types.TagCardinality) ([]string, error) {
	return m.GlobalTagsValue, nil
}

func TestGetTagsOfContainer_RegularContainer(t *testing.T) {
	mockTagger := &MockTagger{
		ContainerTags: map[string][]string{
			"container-123": {"env:prod", "service:web"},
		},
	}

	workloadType, tags, err := GetTagsOfContainer(mockTagger, containerutils.ContainerID("container-123"))

	assert.NoError(t, err)
	assert.Equal(t, WorkloadTypeContainer, workloadType)
	assert.Equal(t, []string{"env:prod", "service:web"}, tags)
}

func TestGetTagsOfContainer_SandboxContainer(t *testing.T) {
	mockTagger := &MockTagger{
		ContainerTags: map[string][]string{}, // No regular container tags
		SandboxTags: map[string][]string{
			"sandbox-456": {"kube_namespace:default", "kube_pod:demo-pod"},
		},
	}

	workloadType, tags, err := GetTagsOfContainer(mockTagger, containerutils.ContainerID("sandbox-456"))

	assert.NoError(t, err)
	assert.Equal(t, WorkloadTypePodSandbox, workloadType)
	assert.Equal(t, []string{"kube_namespace:default", "kube_pod:demo-pod"}, tags)
}

func TestGetTagsOfContainer_UnknownContainer(t *testing.T) {
	mockTagger := &MockTagger{
		ContainerTags: map[string][]string{},
		SandboxTags:   map[string][]string{},
	}

	workloadType, tags, err := GetTagsOfContainer(mockTagger, containerutils.ContainerID("unknown-789"))

	assert.NoError(t, err)
	assert.Equal(t, WorkloadTypeUnknown, workloadType)
	assert.Nil(t, tags)
}

func TestGetTagsOfContainer_NilTagger(t *testing.T) {
	workloadType, tags, err := GetTagsOfContainer(nil, containerutils.ContainerID("container-123"))

	assert.NoError(t, err)
	assert.Equal(t, WorkloadTypeUnknown, workloadType)
	assert.Nil(t, tags)
}

func TestGetTagsOfContainer_TaggerError(t *testing.T) {
	mockTagger := &MockTagger{
		TagError: errors.New("tagger unavailable"),
	}

	workloadType, tags, err := GetTagsOfContainer(mockTagger, containerutils.ContainerID("container-123"))

	assert.Error(t, err)
	assert.Equal(t, WorkloadTypeUnknown, workloadType)
	assert.Nil(t, tags)
}

func TestGetTagsOfContainer_FallbackToSandboxWhenContainerTagsEmpty(t *testing.T) {
	// This test verifies that when a container ID has no tags as a regular container,
	// but has tags as a sandbox container, the sandbox tags are returned.
	mockTagger := &MockTagger{
		ContainerTags: map[string][]string{
			"container-123": {}, // Empty tags for regular container
		},
		SandboxTags: map[string][]string{
			"container-123": {"kube_namespace:kube-system", "kube_pod:coredns"},
		},
	}

	workloadType, tags, err := GetTagsOfContainer(mockTagger, containerutils.ContainerID("container-123"))

	assert.NoError(t, err)
	assert.Equal(t, WorkloadTypePodSandbox, workloadType)
	assert.Equal(t, []string{"kube_namespace:kube-system", "kube_pod:coredns"}, tags)
}

func TestDefaultResolver_Resolve(t *testing.T) {
	mockTagger := &MockTagger{
		ContainerTags: map[string][]string{
			"container-abc": {"env:staging", "version:1.0"},
		},
	}

	resolver := NewDefaultResolver(mockTagger)

	tags := resolver.Resolve(containerutils.ContainerID("container-abc"))
	assert.Equal(t, []string{"env:staging", "version:1.0"}, tags)
}

func TestDefaultResolver_ResolveWithErr(t *testing.T) {
	mockTagger := &MockTagger{
		SandboxTags: map[string][]string{
			"pause-container": {"kube_namespace:test", "kube_pod:nginx"},
		},
	}

	resolver := NewDefaultResolver(mockTagger)

	workloadType, tags, err := resolver.ResolveWithErr(containerutils.ContainerID("pause-container"))
	assert.NoError(t, err)
	assert.Equal(t, WorkloadTypePodSandbox, workloadType)
	assert.Equal(t, []string{"kube_namespace:test", "kube_pod:nginx"}, tags)
}

func TestDefaultResolver_ResolveWithErr_NilWorkloadID(t *testing.T) {
	mockTagger := &MockTagger{}
	resolver := NewDefaultResolver(mockTagger)

	workloadType, tags, err := resolver.ResolveWithErr(nil)
	assert.Error(t, err)
	assert.Equal(t, WorkloadTypeUnknown, workloadType)
	assert.Nil(t, tags)
}

func TestDefaultResolver_ResolveWithErr_EmptyContainerID(t *testing.T) {
	mockTagger := &MockTagger{}
	resolver := NewDefaultResolver(mockTagger)

	workloadType, tags, err := resolver.ResolveWithErr(containerutils.ContainerID(""))
	assert.Error(t, err)
	assert.Equal(t, WorkloadTypeUnknown, workloadType)
	assert.Nil(t, tags)
}

func TestDefaultResolver_GetValue(t *testing.T) {
	mockTagger := &MockTagger{
		ContainerTags: map[string][]string{
			"container-xyz": {"env:production", "service:api", "version:2.0"},
		},
	}

	resolver := NewDefaultResolver(mockTagger)

	value := resolver.GetValue(containerutils.ContainerID("container-xyz"), "env")
	assert.Equal(t, "production", value)

	value = resolver.GetValue(containerutils.ContainerID("container-xyz"), "service")
	assert.Equal(t, "api", value)

	value = resolver.GetValue(containerutils.ContainerID("container-xyz"), "nonexistent")
	assert.Equal(t, "", value)
}
