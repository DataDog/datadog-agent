// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package tags

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
)

func TestStaticTags(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("kubernetes_kubelet_nodename", "eksnode")
	defer mockConfig.SetInTest("kubernetes_kubelet_nodename", "")

	env.SetFeatures(t, env.EKSFargate)

	t.Run("just tags", func(t *testing.T) {
		mockConfig.SetInTest("tags", []string{"some:tag", "another:tag", "nocolon"})
		defer mockConfig.SetInTest("tags", []string{})
		staticTags := GetStaticTags(context.Background(), mockConfig)
		assert.Equal(t, map[string][]string{
			"some":              {"tag"},
			"another":           {"tag"},
			"eks_fargate_node":  {"eksnode"},
			"kube_distribution": {"eks"},
		}, staticTags)
	})

	t.Run("tags and extra_tags", func(t *testing.T) {
		mockConfig.SetInTest("tags", []string{"some:tag", "nocolon"})
		mockConfig.SetInTest("extra_tags", []string{"extra:tag", "missingcolon"})
		defer mockConfig.SetInTest("tags", []string{})
		defer mockConfig.SetInTest("extra_tags", []string{})
		staticTags := GetStaticTags(context.Background(), mockConfig)
		assert.Equal(t, map[string][]string{
			"some":              {"tag"},
			"extra":             {"tag"},
			"eks_fargate_node":  {"eksnode"},
			"kube_distribution": {"eks"},
		}, staticTags)
	})

	t.Run("cluster name already set", func(t *testing.T) {
		mockConfig.SetInTest("tags", []string{"kube_cluster_name:foo"})
		defer mockConfig.SetInTest("tags", []string{})
		staticTags := GetStaticTags(context.Background(), mockConfig)
		assert.Equal(t, map[string][]string{
			"eks_fargate_node":  {"eksnode"},
			"kube_cluster_name": {"foo"},
			"kube_distribution": {"eks"},
		}, staticTags)
	})
}

func TestStaticTagsSlice(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("kubernetes_kubelet_nodename", "eksnode")
	defer mockConfig.SetInTest("kubernetes_kubelet_nodename", "")

	// this test must be kept BEFORE setting eks fargate to test the scenario without EKS fargate set
	t.Run("provider_kind tag without fargate", func(t *testing.T) {
		mockConfig.SetInTest("provider_kind", "gke-autopilot")
		defer mockConfig.SetInTest("provider_kind", "")

		staticTags := GetStaticTagsSlice(context.Background(), mockConfig)
		assert.ElementsMatch(t, []string{"provider_kind:gke-autopilot"}, staticTags)
	})

	env.SetFeatures(t, env.EKSFargate)

	t.Run("just tags", func(t *testing.T) {
		mockConfig.SetInTest("tags", []string{"some:tag", "another:tag", "nocolon"})
		defer mockConfig.SetInTest("tags", []string{})
		staticTags := GetStaticTagsSlice(context.Background(), mockConfig)
		assert.ElementsMatch(t, []string{
			"nocolon",
			"some:tag",
			"another:tag",
			"eks_fargate_node:eksnode",
			"kube_distribution:eks",
		}, staticTags)
	})

	t.Run("tags and extra_tags", func(t *testing.T) {
		mockConfig.SetInTest("tags", []string{"some:tag", "nocolon"})
		mockConfig.SetInTest("extra_tags", []string{"extra:tag", "missingcolon"})
		defer mockConfig.SetInTest("tags", []string{})
		defer mockConfig.SetInTest("extra_tags", []string{})
		staticTags := GetStaticTagsSlice(context.Background(), mockConfig)
		assert.ElementsMatch(t, []string{
			"nocolon",
			"missingcolon",
			"some:tag",
			"extra:tag",
			"eks_fargate_node:eksnode",
			"kube_distribution:eks",
		}, staticTags)
	})
}

func TestClusterAgentGlobalTags(t *testing.T) {
	env.SetFeatures(t, env.Kubernetes)
	clustername.ResetClusterName()
	mockConfig := configmock.New(t)

	// Agent tags config
	mockConfig.SetInTest("tags", []string{"some:tag", "nocolon"})
	mockConfig.SetInTest("extra_tags", []string{"extra:tag", "missingcolon"})
	mockConfig.SetInTest("cluster_checks.extra_tags", []string{"cluster:tag", "nocolon"})
	mockConfig.SetInTest("orchestrator_explorer.extra_tags", []string{"orch:tag", "missingcolon"})

	recordFlavor := flavor.GetFlavor()
	defer func() {
		flavor.SetFlavor(recordFlavor)
	}()

	t.Run("Agent extraGlobalTags", func(t *testing.T) {
		flavor.SetFlavor(flavor.DefaultAgent)
		globalTags := GetClusterAgentStaticTags(t.Context(), mockConfig)
		assert.Equal(t, map[string][]string(nil), globalTags)
	})

	t.Run("ClusterAgent extraGlobalTags", func(t *testing.T) {
		flavor.SetFlavor(flavor.ClusterAgent)
		globalTags := GetClusterAgentStaticTags(t.Context(), mockConfig)
		assert.Equal(t, map[string][]string{
			"some":    {"tag"},
			"extra":   {"tag"},
			"cluster": {"tag"},
			"orch":    {"tag"},
		}, globalTags)
	})
}

func stubEKSIdentity(t *testing.T, tags []string, err error) *int {
	t.Helper()
	original := getClusterAgentEKSIdentityTags
	t.Cleanup(func() { getClusterAgentEKSIdentityTags = original })
	calls := new(int)
	getClusterAgentEKSIdentityTags = func(context.Context) ([]string, error) {
		*calls++
		return tags, err
	}
	return calls
}

var eksIdentityTags = []string{
	"eks_cluster_arn:arn:aws:eks:us-west-2:123456789012:cluster/orders",
	"aws_account:123456789012",
	"region:us-west-2",
}

func TestStaticTagsSliceEKSIdentityOnNodeAgent(t *testing.T) {
	env.SetFeatures(t, env.Kubernetes)
	recordFlavor := flavor.GetFlavor()
	t.Cleanup(func() { flavor.SetFlavor(recordFlavor) })
	flavor.SetFlavor(flavor.DefaultAgent)

	mockConfig := configmock.New(t)

	t.Run("cluster agent disabled does not query identity", func(t *testing.T) {
		mockConfig.SetInTest("cluster_agent.enabled", false)
		calls := stubEKSIdentity(t, eksIdentityTags, nil)
		assert.Empty(t, GetStaticTagsSlice(t.Context(), mockConfig))
		assert.Equal(t, 0, *calls)
	})

	t.Run("identity from cluster agent is attached", func(t *testing.T) {
		mockConfig.SetInTest("cluster_agent.enabled", true)
		calls := stubEKSIdentity(t, eksIdentityTags, nil)
		assert.ElementsMatch(t, eksIdentityTags, GetStaticTagsSlice(t.Context(), mockConfig))
		assert.Equal(t, 1, *calls)
	})

	t.Run("identity failure adds nothing", func(t *testing.T) {
		mockConfig.SetInTest("cluster_agent.enabled", true)
		stubEKSIdentity(t, nil, errors.New("cluster is not EKS"))
		assert.Empty(t, GetStaticTagsSlice(t.Context(), mockConfig))
	})

	t.Run("cluster agent flavor never queries itself", func(t *testing.T) {
		flavor.SetFlavor(flavor.ClusterAgent)
		defer flavor.SetFlavor(flavor.DefaultAgent)
		mockConfig.SetInTest("cluster_agent.enabled", true)
		calls := stubEKSIdentity(t, eksIdentityTags, nil)
		assert.Empty(t, GetStaticTagsSlice(t.Context(), mockConfig))
		assert.Equal(t, 0, *calls)
	})
}

func TestStaticTagsSliceEKSIdentityOnFargate(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetInTest("kubernetes_kubelet_nodename", "eksnode")
	mockConfig.SetInTest("cluster_agent.enabled", true)
	env.SetFeatures(t, env.EKSFargate)

	originalStatic := getClusterAgentStaticTags
	t.Cleanup(func() { getClusterAgentStaticTags = originalStatic })
	getClusterAgentStaticTags = func() ([]string, error) {
		return []string{"orch_cluster_id:94e43011-177b-11ea-a4fe-42010a8401d2"}, nil
	}

	t.Run("identity is attached to sidecar static tags", func(t *testing.T) {
		stubEKSIdentity(t, eksIdentityTags, nil)
		staticTags := GetStaticTagsSlice(t.Context(), mockConfig)
		assert.Subset(t, staticTags, eksIdentityTags)
		assert.Contains(t, staticTags, "orch_cluster_id:94e43011-177b-11ea-a4fe-42010a8401d2")
		assert.Contains(t, staticTags, "kube_distribution:eks")
	})

	t.Run("identity failure keeps existing sidecar tags", func(t *testing.T) {
		stubEKSIdentity(t, nil, errors.New("unavailable"))
		staticTags := GetStaticTagsSlice(t.Context(), mockConfig)
		assert.Contains(t, staticTags, "kube_distribution:eks")
		for _, tag := range eksIdentityTags {
			assert.NotContains(t, staticTags, tag)
		}
	})
}

func TestClusterAgentGlobalTagsEKSIdentity(t *testing.T) {
	env.SetFeatures(t, env.Kubernetes)
	clustername.ResetClusterName()
	mockConfig := configmock.New(t)
	recordFlavor := flavor.GetFlavor()
	t.Cleanup(func() { flavor.SetFlavor(recordFlavor) })
	flavor.SetFlavor(flavor.ClusterAgent)

	original := getClusterAgentEKSIdentity
	t.Cleanup(func() { getClusterAgentEKSIdentity = original })
	calls := 0
	getClusterAgentEKSIdentity = func(context.Context) []string {
		calls++
		return eksIdentityTags
	}

	// Without EKS detection the resolver must not be consulted at all.
	globalTags := GetClusterAgentStaticTags(t.Context(), mockConfig)
	assert.Equal(t, 0, calls)
	assert.NotContains(t, globalTags, "eks_cluster_arn")
}
