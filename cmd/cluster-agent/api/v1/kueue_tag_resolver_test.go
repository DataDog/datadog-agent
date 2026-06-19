// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package v1

import (
	"testing"

	"github.com/stretchr/testify/assert"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestResolveQueueMetadataAsTags(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("kubernetes_resources_labels_as_tags",
		`{"localqueues.kueue.x-k8s.io": {"team": "team", "owner": "+owner"}}`)
	cfg.SetInTest("kubernetes_resources_annotations_as_tags",
		`{"localqueues.kueue.x-k8s.io": {"cost-center": "cost_center"}}`)

	resolver := newKueueResourcesMetadataAsTagsResolver(cfg)

	queue := &workloadmeta.KubernetesKueueQueue{
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "batch",
			Namespace: "default",
			Labels: map[string]string{
				"team":      "eng",
				"owner":     "alice",
				"unrelated": "skip",
			},
			Annotations: map[string]string{
				"cost-center": "1234",
			},
		},
		QueueType: workloadmeta.KueueLocalQueue,
	}

	tags := resolver.resolveQueueMetadataAsTags(queue)

	// High-cardinality tags are prefixed with '+'; labels/annotations without a
	// matching configuration entry are dropped.
	assert.ElementsMatch(t, []string{
		"team:eng",
		"+owner:alice",
		"cost_center:1234",
	}, tags)
}

func TestResolveQueueMetadataAsTagsNoConfig(t *testing.T) {
	cfg := configmock.New(t)
	resolver := newKueueResourcesMetadataAsTagsResolver(cfg)

	queue := &workloadmeta.KubernetesKueueQueue{
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "batch",
			Labels: map[string]string{"team": "eng"},
		},
		QueueType: workloadmeta.KueueLocalQueue,
	}

	assert.Nil(t, resolver.resolveQueueMetadataAsTags(queue))
}

func TestResolveQueueMetadataAsTagsClusterQueueGroupResource(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("kubernetes_resources_labels_as_tags",
		`{"clusterqueues.kueue.x-k8s.io": {"tier": "tier"}}`)

	resolver := newKueueResourcesMetadataAsTagsResolver(cfg)

	queue := &workloadmeta.KubernetesKueueQueue{
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "cluster-batch",
			Labels: map[string]string{"tier": "gold"},
		},
		QueueType: workloadmeta.KueueClusterQueue,
	}

	assert.ElementsMatch(t, []string{"tier:gold"}, resolver.resolveQueueMetadataAsTags(queue))
}

func TestResolveResourceFlavorMetadataAsTags(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("kubernetes_resources_labels_as_tags",
		`{"resourceflavors.kueue.x-k8s.io": {"flavor_class": "flavor", "owner": "+owner"}}`)
	cfg.SetInTest("kubernetes_resources_annotations_as_tags",
		`{"resourceflavors.kueue.x-k8s.io": {"cost-center": "cost_center"}}`)

	resolver := newKueueResourcesMetadataAsTagsResolver(cfg)

	flavor := &workloadmeta.KubernetesKueueResourceFlavor{
		EntityMeta: workloadmeta.EntityMeta{
			Name: "a100",
			Labels: map[string]string{
				"flavor_class": "gpu",
				"owner":        "alice",
				"unrelated":    "skip",
			},
			Annotations: map[string]string{
				"cost-center": "1234",
			},
		},
	}

	assert.ElementsMatch(t, []string{
		"flavor:gpu",
		"+owner:alice",
		"cost_center:1234",
	}, resolver.resolveResourceFlavorMetadataAsTags(flavor))
}
