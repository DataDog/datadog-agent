// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package kubernetesresourceparsers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestKueueQueueParser(t *testing.T) {
	tests := []struct {
		name      string
		queueType workloadmeta.KueueQueueType
		obj       *unstructured.Unstructured
		expected  *workloadmeta.KubernetesKueueQueue
	}{
		{
			name:      "local queue",
			queueType: workloadmeta.KueueLocalQueue,
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name":        "local-a",
						"namespace":   "team-a",
						"labels":      map[string]interface{}{"team": "a"},
						"annotations": map[string]interface{}{"owner": "batch"},
						"uid":         "uid-local-a",
					},
					"spec": map[string]interface{}{
						"clusterQueue": "cluster-a",
					},
				},
			},
			expected: &workloadmeta.KubernetesKueueQueue{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesKueueQueue,
					ID:   "localqueue/team-a/local-a",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:        "local-a",
					Namespace:   "team-a",
					Labels:      map[string]string{"team": "a"},
					Annotations: map[string]string{"owner": "batch"},
					UID:         "uid-local-a",
				},
				QueueType:        workloadmeta.KueueLocalQueue,
				ClusterQueueName: "cluster-a",
			},
		},
		{
			name:      "cluster queue",
			queueType: workloadmeta.KueueClusterQueue,
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "cluster-a",
						"uid":  "uid-cluster-a",
					},
				},
			},
			expected: &workloadmeta.KubernetesKueueQueue{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesKueueQueue,
					ID:   "clusterqueue//cluster-a",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: "cluster-a",
					UID:  "uid-cluster-a",
				},
				QueueType:        workloadmeta.KueueClusterQueue,
				ClusterQueueName: "cluster-a",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser := NewKueueQueueParser(test.queueType)
			assert.Equal(t, test.expected, parser.Parse(test.obj))
		})
	}
}
