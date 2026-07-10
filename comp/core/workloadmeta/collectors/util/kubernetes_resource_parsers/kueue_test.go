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
			parser, err := NewKueueQueueParser(test.queueType)
			assert.NoError(t, err)
			assert.Equal(t, test.expected, parser.Parse(test.obj))
		})
	}
}

func TestKueueResourceFlavorParser(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":        "a100",
				"labels":      map[string]interface{}{"team": "ml"},
				"annotations": map[string]interface{}{"owner": "batch"},
				"uid":         "uid-a100",
			},
			"spec": map[string]interface{}{
				"nodeLabels": map[string]interface{}{
					"nvidia.com/gpu.product": "NVIDIA-A100-SXM4-40GB",
				},
			},
		},
	}

	expected := &workloadmeta.KubernetesKueueResourceFlavor{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesKueueResourceFlavor,
			ID:   "a100",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        "a100",
			Labels:      map[string]string{"team": "ml"},
			Annotations: map[string]string{"owner": "batch"},
			UID:         "uid-a100",
		},
		NodeAffinityLabels: map[string]string{
			"nvidia.com/gpu.product": "NVIDIA-A100-SXM4-40GB",
		},
	}

	parser := NewKueueResourceFlavorParser()
	assert.Equal(t, expected, parser.Parse(obj))
}

func TestKueueWorkloadParser(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":        "job-sample-df83f",
				"namespace":   "team-a",
				"labels":      map[string]interface{}{"team": "a"},
				"annotations": map[string]interface{}{"owner": "batch"},
				"uid":         "uid-workload",
			},
			"spec": map[string]interface{}{
				"queueName": "gpu",
			},
			"status": map[string]interface{}{
				"admission": map[string]interface{}{
					"clusterQueue": "team-a-gpu",
					"podSetAssignments": []interface{}{
						map[string]interface{}{
							"name": "main",
							"flavors": map[string]interface{}{
								"nvidia.com/gpu": "a100",
								"cpu":            "default",
							},
						},
					},
				},
			},
		},
	}

	expected := &workloadmeta.KubernetesKueueWorkload{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesKueueWorkload,
			ID:   "team-a/job-sample-df83f",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        "job-sample-df83f",
			Namespace:   "team-a",
			Labels:      map[string]string{"team": "a"},
			Annotations: map[string]string{"owner": "batch"},
			UID:         "uid-workload",
		},
		QueueName:        "gpu",
		ClusterQueueName: "team-a-gpu",
		PodSetAssignments: []workloadmeta.KueuePodSetAssignment{
			{
				Name: "main",
				Flavors: map[string]string{
					"nvidia.com/gpu": "a100",
					"cpu":            "default",
				},
			},
		},
	}

	parser := NewKueueWorkloadParser()
	assert.Equal(t, expected, parser.Parse(obj))
}

func TestGenerateKueueQueueEntityID(t *testing.T) {
	tests := []struct {
		name        string
		queueType   workloadmeta.KueueQueueType
		namespace   string
		queueName   string
		expectedID  string
		expectedErr string
	}{
		{
			name:       "local queue",
			queueType:  workloadmeta.KueueLocalQueue,
			namespace:  "team-a",
			queueName:  "local-a",
			expectedID: "localqueue/team-a/local-a",
		},
		{
			name:       "cluster queue",
			queueType:  workloadmeta.KueueClusterQueue,
			namespace:  "ignored",
			queueName:  "cluster-a",
			expectedID: "clusterqueue//cluster-a",
		},
		{
			name:        "unsupported queue type",
			queueType:   workloadmeta.KueueQueueType("cohort"),
			namespace:   "team-a",
			queueName:   "cohort-a",
			expectedErr: `unsupported Kueue queue type "cohort"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entityID, err := GenerateKueueQueueEntityID(test.queueType, test.namespace, test.queueName)
			if test.expectedErr != "" {
				assert.EqualError(t, err, test.expectedErr)
				assert.Empty(t, entityID)
				return
			}
			assert.NoError(t, err)
			assert.Equal(t, test.expectedID, entityID)
		})
	}
}

func TestNewKueueQueueParserUnsupportedQueueType(t *testing.T) {
	parser, err := NewKueueQueueParser(workloadmeta.KueueQueueType("cohort"))
	assert.EqualError(t, err, `unsupported Kueue queue type "cohort"`)
	assert.Nil(t, parser)
}
