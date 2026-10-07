// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestParseOriginalResources(t *testing.T) {
	original, ok := parseOriginalResources(nil, true)
	require.True(t, ok, "no annotation is a valid empty record")
	assert.Empty(t, original)

	original, ok = parseOriginalResources(map[string]string{model.OriginalResourcesAnnotation: `{"app":{"requests":{"cpu":"100m"},"limits":{"memory":null}}}`}, true)
	require.True(t, ok)
	require.Contains(t, original, "app")
	require.NotNil(t, original["app"].Requests[corev1.ResourceCPU])
	assert.True(t, original["app"].Requests[corev1.ResourceCPU].Equal(resource.MustParse("100m")))
	value, recorded := original["app"].Limits[corev1.ResourceMemory]
	assert.True(t, recorded, "an absent field is recorded")
	assert.Nil(t, value)

	original, ok = parseOriginalResources(map[string]string{model.OriginalResourcesAnnotation: `not json`}, false)
	assert.False(t, ok, "an invalid record is not used")
	assert.Nil(t, original)

	// A managed pod without a record may have values the autoscaler already changed.
	for name, annotations := range map[string]map[string]string{
		"attributed to an autoscaler": {model.AutoscalerIDAnnotation: "default/dpa"},
		"recommendation applied":      {model.RecommendationIDAnnotation: "r1"},
	} {
		t.Run(name, func(t *testing.T) {
			original, ok := parseOriginalResources(annotations, true)
			assert.False(t, ok, "unknown when the record is required")
			assert.Nil(t, original)
			original, ok = parseOriginalResources(annotations, false)
			assert.True(t, ok, "empty at admission, where the pod is built from its template")
			assert.Empty(t, original)
		})
	}
}

func TestOriginalResourcesMerge(t *testing.T) {
	live := originalResources{}
	live.record("app", true, corev1.ResourceMemory, pointer.Ptr(resource.MustParse("64Mi")))

	snapshot := originalResources{}
	snapshot.record("app", true, corev1.ResourceMemory, pointer.Ptr(resource.MustParse("32Mi"))) // stale: recorded later, from a changed value
	snapshot.record("app", false, corev1.ResourceCPU, nil)

	assert.True(t, live.merge(snapshot))
	encoded, err := live.encode()
	require.NoError(t, err)
	assert.JSONEq(t, `{"app":{"requests":{"cpu":null},"limits":{"memory":"64Mi"}}}`, encoded, "live entries win, new ones are added")
	assert.False(t, live.merge(snapshot), "nothing new")
}

func TestOriginalResourcesRecord(t *testing.T) {
	original := originalResources{}
	assert.True(t, original.record("app", true, corev1.ResourceMemory, pointer.Ptr(resource.MustParse("64Mi"))))
	assert.False(t, original.record("app", true, corev1.ResourceMemory, pointer.Ptr(resource.MustParse("128Mi"))), "first write wins")
	assert.True(t, original.record("app", false, corev1.ResourceCPU, nil), "an absent field is recorded")

	encoded, err := original.encode()
	require.NoError(t, err)
	assert.JSONEq(t, `{"app":{"requests":{"cpu":null},"limits":{"memory":"64Mi"}}}`, encoded)

	parsed, ok := parseOriginalResources(map[string]string{model.OriginalResourcesAnnotation: encoded}, true)
	require.True(t, ok)
	reencoded, err := parsed.encode()
	require.NoError(t, err)
	assert.JSONEq(t, encoded, reencoded, "the record round-trips")
}

func TestOriginalResourcesRecordChanges(t *testing.T) {
	before := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("64Mi")},
		Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("200m")},
	}
	after := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")}, // cpu unchanged
		Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("256Mi")},                                                 // cpu removed, memory added
	}

	original := originalResources{}
	require.True(t, original.recordChanges("app", before, after))
	encoded, err := original.encode()
	require.NoError(t, err)
	assert.JSONEq(t, `{"app":{"requests":{"memory":"64Mi"},"limits":{"cpu":"200m","memory":null}}}`, encoded)

	assert.False(t, original.recordChanges("app", after, after), "nothing changes")
}
