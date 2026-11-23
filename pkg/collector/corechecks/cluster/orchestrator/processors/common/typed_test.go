// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
)

func TestTypedHandlers(t *testing.T) {
	objectMeta := func() metav1.ObjectMeta {
		return metav1.ObjectMeta{
			Name:            "name",
			Namespace:       "default",
			UID:             types.UID("uid"),
			ResourceVersion: "42",
			Labels:          map[string]string{"app": "name"},
			Annotations: map[string]string{
				"annotation": "value",
				"kubectl.kubernetes.io/last-applied-configuration": "secret",
			},
		}
	}

	t.Run("typed resource", func(t *testing.T) {
		testTypedHandlers(t, &appsv1.Deployment{ObjectMeta: objectMeta()}, "apps/v1", "Deployment")
	})
	t.Run("core resource", func(t *testing.T) {
		testTypedHandlers(t, &corev1.Service{ObjectMeta: objectMeta()}, "v1", "Service")
	})
	t.Run("metadata-only resource", func(t *testing.T) {
		testTypedHandlers(t, &metav1.PartialObjectMetadata{ObjectMeta: objectMeta()}, "v1", "ConfigMap")
	})
}

func testTypedHandlers[P TypedObject[P]](t *testing.T, resource P, apiVersion, kind string) {
	var h TypedHandlers[P]
	ctx := &processors.K8sProcessorContext{BaseProcessorContext: processors.BaseProcessorContext{APIVersion: apiVersion, Kind: kind}}

	assert.Equal(t, []interface{}{resource}, h.ResourceList(ctx, []P{resource}))
	assert.Equal(t, types.UID("uid"), h.ResourceUID(ctx, resource))
	assert.Equal(t, "42", h.ResourceVersion(ctx, resource, nil))
	assert.Equal(t, "42", h.ResourceVersionFromRaw(ctx, resource))

	clone, ok := h.CloneResource(resource).(P)
	require.True(t, ok)
	assert.Equal(t, resource, clone)
	assert.NotSame(t, resource, clone)

	assert.False(t, h.BeforeMarshalling(ctx, clone, nil))
	gvk := clone.GetObjectKind().GroupVersionKind()
	assert.Equal(t, apiVersion, gvk.GroupVersion().String())
	assert.Equal(t, kind, gvk.Kind)
	assert.Empty(t, resource.GetObjectKind().GroupVersionKind().Kind, "the clone is independent of the resource")

	h.ScrubBeforeExtraction(ctx, clone)
	assert.Equal(t, map[string]string{"annotation": "value", "kubectl.kubernetes.io/last-applied-configuration": "-"}, clone.GetAnnotations())
	assert.Equal(t, map[string]string{"app": "name"}, clone.GetLabels())
}
