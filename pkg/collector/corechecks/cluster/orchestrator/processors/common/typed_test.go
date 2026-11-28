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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
	t.Run("unstructured resource", func(t *testing.T) {
		// Unstructured objects return copies of their label and annotation maps.
		meta := objectMeta()
		resource := &unstructured.Unstructured{Object: map[string]interface{}{}}
		resource.SetName(meta.Name)
		resource.SetNamespace(meta.Namespace)
		resource.SetUID(meta.UID)
		resource.SetResourceVersion(meta.ResourceVersion)
		resource.SetLabels(meta.Labels)
		resource.SetAnnotations(meta.Annotations)
		testTypedHandlers(t, resource, "datadoghq.com/v1alpha1", "DatadogMetric")
	})
}

func testTypedHandlers[P TypedObject[P]](t *testing.T, resource P, apiVersion, kind string) {
	var h TypedHandlers[P]
	ctx := &processors.K8sProcessorContext{BaseProcessorContext: processors.BaseProcessorContext{APIVersion: apiVersion, Kind: kind}}

	type typedResults struct {
		List                 []interface{}
		UID                  types.UID
		Version, RawVersion  string
		CloneEqual, CloneNew bool
		Skip                 bool
		APIVersion, Kind     string
		ResourceKind         string
		Annotations, Labels  map[string]string
	}

	clone, ok := h.CloneResource(resource).(P)
	require.True(t, ok)
	got := typedResults{
		List:       h.ResourceList(ctx, []P{resource}),
		UID:        h.ResourceUID(ctx, resource),
		Version:    h.ResourceVersion(ctx, resource, nil),
		RawVersion: h.ResourceVersionFromRaw(ctx, resource),
		CloneEqual: assert.ObjectsAreEqual(resource, clone),
		CloneNew:   any(clone) != any(resource),
		Skip:       h.BeforeMarshalling(ctx, clone, nil),
	}
	h.ScrubBeforeExtraction(ctx, clone)
	gvk := clone.GetObjectKind().GroupVersionKind()
	got.APIVersion, got.Kind = gvk.GroupVersion().String(), gvk.Kind
	got.ResourceKind = resource.GetObjectKind().GroupVersionKind().Kind
	got.Annotations, got.Labels = clone.GetAnnotations(), clone.GetLabels()

	assert.Equal(t, typedResults{
		List:        []interface{}{resource},
		UID:         types.UID("uid"),
		Version:     "42",
		RawVersion:  "42",
		CloneEqual:  true,
		CloneNew:    true,
		APIVersion:  apiVersion,
		Kind:        kind,
		Annotations: map[string]string{"annotation": "value", "kubectl.kubernetes.io/last-applied-configuration": "-"},
		Labels:      map[string]string{"app": "name"},
	}, got, "ResourceKind stays empty: the clone is independent of the resource")
}
