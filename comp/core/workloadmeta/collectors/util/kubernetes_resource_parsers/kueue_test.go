// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package kubernetesresourceparsers

import (
	"testing"
	"time"

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
	priority := int32(100)
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":              "job-sample-df83f",
				"namespace":         "team-a",
				"labels":            map[string]interface{}{"team": "a"},
				"annotations":       map[string]interface{}{"owner": "batch"},
				"uid":               "uid-workload",
				"creationTimestamp": "2026-10-14T14:00:00Z",
			},
			"spec": map[string]interface{}{
				"queueName":        "gpu",
				"priority":         int64(100),
				"priorityClassRef": map[string]interface{}{"name": "high"},
				"podSets": []interface{}{
					map[string]interface{}{
						"name": "driver",
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"team":       "ml",
									"spark-role": "driver",
								},
								"annotations": map[string]interface{}{
									"owner": "platform",
									"note":  "driver",
								},
							},
						},
					},
					map[string]interface{}{
						"name": "executor",
						"template": map[string]interface{}{
							"metadata": map[string]interface{}{
								"labels": map[string]interface{}{
									"team":       "ml",
									"spark-role": "executor",
								},
								"annotations": map[string]interface{}{
									"owner": "platform",
									"note":  "executor",
								},
							},
						},
					},
				},
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":               "QuotaReserved",
						"status":             "True",
						"reason":             "QuotaReserved",
						"message":            "Quota reserved in ClusterQueue team-a-gpu",
						"lastTransitionTime": "2026-10-14T14:01:00Z",
					},
					map[string]interface{}{"status": "True"},
				},
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
		QueueName:         "gpu",
		ClusterQueueName:  "team-a-gpu",
		CreationTimestamp: time.Date(2026, 10, 14, 14, 0, 0, 0, time.UTC),
		Priority:          &priority,
		PriorityClassName: "high",
		Conditions: []workloadmeta.KueueWorkloadCondition{
			{
				Type:               "QuotaReserved",
				Status:             "True",
				Reason:             "QuotaReserved",
				Message:            "Quota reserved in ClusterQueue team-a-gpu",
				LastTransitionTime: time.Date(2026, 10, 14, 14, 1, 0, 0, time.UTC),
			},
		},
		PodTemplateLabels: map[string]string{
			"team": "ml",
		},
		PodTemplateAnnotations: map[string]string{
			"owner": "platform",
		},
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

	parser := NewKueueWorkloadParser(true, true)
	assert.Equal(t, expected, parser.Parse(obj))

	disabled := NewKueueWorkloadParser(false, true)
	expected.PodTemplateLabels = nil
	expected.PodTemplateAnnotations = nil
	assert.Equal(t, expected, disabled.Parse(obj))
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

func TestKueueWorkloadParserDeprecatedPriorityClassName(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{"name": "wl", "namespace": "ns"},
			"spec":     map[string]interface{}{"priorityClassName": "legacy"},
		},
	}

	workload := NewKueueWorkloadParser(false, true).Parse(obj).(*workloadmeta.KubernetesKueueWorkload)

	assert.Equal(t, "legacy", workload.PriorityClassName)
	assert.Nil(t, workload.Priority)
	assert.True(t, workload.CreationTimestamp.IsZero())
	assert.Empty(t, workload.Conditions)
}

func TestParseRFC3339(t *testing.T) {
	assert.True(t, parseRFC3339("").IsZero())
	assert.True(t, parseRFC3339("yesterday").IsZero(), "a malformed value gives the zero time")
	assert.Equal(t, time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC), parseRFC3339("2026-11-19T10:00:00Z"))
}

func TestKueueWorkloadParserWithoutEventsLeavesOutConditions(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"metadata": map[string]interface{}{"name": "job-1", "namespace": "team-a", "uid": "uid-1"},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Admitted", "status": "True", "lastTransitionTime": "2026-11-19T10:00:00Z"},
			},
		},
	}}

	with := NewKueueWorkloadParser(false, true).Parse(obj).(*workloadmeta.KubernetesKueueWorkload)
	assert.Len(t, with.Conditions, 1)

	without := NewKueueWorkloadParser(false, false).Parse(obj).(*workloadmeta.KubernetesKueueWorkload)
	assert.Empty(t, without.Conditions)
}
