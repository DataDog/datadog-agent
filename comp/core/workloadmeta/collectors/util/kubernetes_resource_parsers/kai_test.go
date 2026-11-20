// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package kubernetesresourceparsers

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestKAIPodGroupParserFixtures(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "kai", "podgroups.yaml"))
	require.NoError(t, err)
	jsonList, err := yaml.YAMLToJSON(raw)
	require.NoError(t, err)
	var list unstructured.UnstructuredList
	require.NoError(t, list.UnmarshalJSON(jsonList))
	require.NotEmpty(t, list.Items)

	podGroup := NewKAIPodGroupParser(true).Parse(&list.Items[0]).(*workloadmeta.KubernetesKAIPodGroup)
	assert.Equal(t, &workloadmeta.KubernetesKAIPodGroup{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesKAIPodGroup,
			ID:   "kai-test/pg-frac-a-efc71a63-bdb9-4f0b-b95a-7309efddf661",
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      "pg-frac-a-efc71a63-bdb9-4f0b-b95a-7309efddf661",
			Namespace: "kai-test",
			Labels:    map[string]string{"kai.scheduler/queue": "default-queue"},
			Annotations: map[string]string{
				"gpu-fraction":                     "0.5",
				"kai.scheduler/top-owner-metadata": "name: frac-a\nuid: efc71a63-bdb9-4f0b-b95a-7309efddf661\ngroup: \"\"\nversion: v1\nkind: Pod\n",
				"runai/shared-gpu-configmap":       "frac-a-jb6btbp-shared-gpu",
			},
			UID: "355f982a-d7b8-4ef4-ab6e-232bb97a6ad2",
		},
		QueueName:         "default-queue",
		CreationTimestamp: time.Date(2026, 10, 14, 14, 49, 10, 0, time.UTC),
		PriorityClassName: "train",
		LastStartTime:     time.Date(2026, 10, 14, 14, 49, 10, 0, time.UTC),
	}, podGroup)
}

func TestKAIPodGroupParserSchedulingState(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "pg",
			"namespace": "ns",
			"annotations": map[string]interface{}{
				"kai.scheduler/last-start-timestamp":    "2026-10-14T14:49:10Z",
				"kai.scheduler/last-eviction-timestamp": "2026-10-14T14:50:00Z",
			},
		},
		"spec": map[string]interface{}{"queue": "research"},
		"status": map[string]interface{}{
			"schedulingConditions": []interface{}{
				map[string]interface{}{
					"type":               "UnschedulableOnNodePool",
					"status":             "True",
					"nodePool":           "gpu",
					"message":            "not enough GPUs",
					"lastTransitionTime": "2026-10-14T14:49:30Z",
					"reasons": []interface{}{
						map[string]interface{}{"reason": "OverLimit", "message": "queue limit reached"},
						"not an object",
					},
				},
				map[string]interface{}{"status": "True"},
				"not an object",
			},
		},
	}

	podGroup := NewKAIPodGroupParser(true).Parse(&unstructured.Unstructured{Object: obj}).(*workloadmeta.KubernetesKAIPodGroup)

	assert.Equal(t, time.Date(2026, 10, 14, 14, 49, 10, 0, time.UTC), podGroup.LastStartTime)
	assert.Equal(t, time.Date(2026, 10, 14, 14, 50, 0, 0, time.UTC), podGroup.LastEvictionTime)
	assert.Equal(t, []workloadmeta.KAISchedulingCondition{
		{
			Type:     "UnschedulableOnNodePool",
			Status:   "True",
			NodePool: "gpu",
			Message:  "not enough GPUs",
			Explanations: []workloadmeta.KAISchedulingExplanation{
				{Reason: "OverLimit", Message: "queue limit reached"},
			},
			LastTransitionTime: time.Date(2026, 10, 14, 14, 49, 30, 0, time.UTC),
		},
	}, podGroup.SchedulingConditions)

	// The volatile annotation stays out of the metadata that is streamed to the nodes.
	assert.NotContains(t, podGroup.Annotations, "kai.scheduler/last-start-timestamp")
}

func TestKAIPodGroupParserMalformedTimes(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name":      "pg",
			"namespace": "ns",
			"annotations": map[string]interface{}{
				"kai.scheduler/last-start-timestamp": "yesterday",
			},
		},
		"spec": map[string]interface{}{"queue": "research"},
	}

	podGroup := NewKAIPodGroupParser(true).Parse(&unstructured.Unstructured{Object: obj}).(*workloadmeta.KubernetesKAIPodGroup)

	assert.True(t, podGroup.LastStartTime.IsZero())
	assert.True(t, podGroup.LastEvictionTime.IsZero())
	assert.Empty(t, podGroup.SchedulingConditions)
}

func TestKAIPodGroupParserInvalidQueue(t *testing.T) {
	tests := []struct {
		name string
		spec map[string]interface{}
	}{
		{name: "no queue", spec: map[string]interface{}{"minMember": int64(1)}},
		{name: "queue is not a string", spec: map[string]interface{}{"queue": int64(1)}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			obj := map[string]interface{}{
				"metadata": map[string]interface{}{
					"name":      "pg",
					"namespace": "ns",
				},
				"spec": test.spec,
			}

			podGroup := NewKAIPodGroupParser(true).Parse(&unstructured.Unstructured{Object: obj}).(*workloadmeta.KubernetesKAIPodGroup)
			assert.Equal(t, "ns/pg", podGroup.ID)
			assert.Empty(t, podGroup.QueueName)
		})
	}
}

func TestKAIPodGroupParserWithoutEventsLeavesOutStatusFields(t *testing.T) {
	obj := map[string]interface{}{
		"metadata": map[string]interface{}{
			"name": "pg", "namespace": "ns", "uid": "uid-1",
			"annotations": map[string]interface{}{
				kaiLastStartAnnotation:    "2026-11-19T10:00:00Z",
				kaiLastEvictionAnnotation: "2026-11-19T10:05:00Z",
			},
		},
		"status": map[string]interface{}{
			"schedulingConditions": []interface{}{
				map[string]interface{}{"type": "Unschedulable", "status": "True", "nodePool": "default"},
			},
		},
	}

	with := NewKAIPodGroupParser(true).Parse(&unstructured.Unstructured{Object: obj}).(*workloadmeta.KubernetesKAIPodGroup)
	assert.False(t, with.LastStartTime.IsZero())
	assert.False(t, with.LastEvictionTime.IsZero())
	assert.Len(t, with.SchedulingConditions, 1)

	without := NewKAIPodGroupParser(false).Parse(&unstructured.Unstructured{Object: obj}).(*workloadmeta.KubernetesKAIPodGroup)
	assert.True(t, without.LastStartTime.IsZero())
	assert.True(t, without.LastEvictionTime.IsZero())
	assert.Empty(t, without.SchedulingConditions)
	assert.NotContains(t, without.Annotations, kaiLastStartAnnotation)
}
