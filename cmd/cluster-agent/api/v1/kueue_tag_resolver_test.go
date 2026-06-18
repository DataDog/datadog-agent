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

func TestResolveKueueQueueTags(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("kubernetes_resources_labels_as_tags",
		`{"localqueues.kueue.x-k8s.io": {"team": "team", "owner": "+owner"}}`)
	cfg.SetInTest("kubernetes_resources_annotations_as_tags",
		`{"localqueues.kueue.x-k8s.io": {"cost-center": "cost_center"}}`)

	resolver := newKueueQueueTagResolver(cfg)

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

	tags := resolver.resolveKueueQueueTags(queue)

	// High-cardinality tags are prefixed with '+'; labels/annotations without a
	// matching configuration entry are dropped.
	assert.ElementsMatch(t, []string{
		"team:eng",
		"+owner:alice",
		"cost_center:1234",
	}, tags)
}

func TestResolveKueueQueueTagsNoConfig(t *testing.T) {
	cfg := configmock.New(t)
	resolver := newKueueQueueTagResolver(cfg)

	queue := &workloadmeta.KubernetesKueueQueue{
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "batch",
			Labels: map[string]string{"team": "eng"},
		},
		QueueType: workloadmeta.KueueLocalQueue,
	}

	assert.Nil(t, resolver.resolveKueueQueueTags(queue))
}

func TestResolveKueueQueueTagsClusterQueueGroupResource(t *testing.T) {
	cfg := configmock.New(t)
	cfg.SetInTest("kubernetes_resources_labels_as_tags",
		`{"clusterqueues.kueue.x-k8s.io": {"tier": "tier"}}`)

	resolver := newKueueQueueTagResolver(cfg)

	queue := &workloadmeta.KubernetesKueueQueue{
		EntityMeta: workloadmeta.EntityMeta{
			Name:   "cluster-batch",
			Labels: map[string]string{"tier": "gold"},
		},
		QueueType: workloadmeta.KueueClusterQueue,
	}

	assert.ElementsMatch(t, []string{"tier:gold"}, resolver.resolveKueueQueueTags(queue))
}
