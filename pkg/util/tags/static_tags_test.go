// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package tags

import (
	"context"
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
