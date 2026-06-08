// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

func minimalProfile(name string) *datadoghq.DatadogPodAutoscalerClusterProfile {
	maxReplicas := int32(5)
	return &datadoghq.DatadogPodAutoscalerClusterProfile{
		ObjectMeta: metav1.ObjectMeta{Name: name, Generation: 1},
		Spec: datadoghq.DatadogPodAutoscalerProfileSpec{
			Template: datadoghq.DatadogPodAutoscalerTemplate{
				Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{MaxReplicas: &maxReplicas},
			},
		},
	}
}

func TestPodAutoscalerProfileInternal_PreviewAnnotation(t *testing.T) {
	t.Run("empty by default (no annotation)", func(t *testing.T) {
		profile := minimalProfile("p1")
		pi, err := NewPodAutoscalerProfileInternal(profile)
		require.NoError(t, err)
		assert.Empty(t, pi.PreviewAnnotation())
	})

	t.Run("forwards preview annotation when burstable:true", func(t *testing.T) {
		profile := minimalProfile("p1")
		profile.Annotations = map[string]string{PreviewAnnotationKey: `{"burstable":true}`}
		pi, err := NewPodAutoscalerProfileInternal(profile)
		require.NoError(t, err)
		assert.Equal(t, `{"burstable":true}`, pi.PreviewAnnotation())
	})

	t.Run("forwards preview annotation when burstable:false", func(t *testing.T) {
		profile := minimalProfile("p1")
		profile.Annotations = map[string]string{PreviewAnnotationKey: `{"burstable":false}`}
		pi, err := NewPodAutoscalerProfileInternal(profile)
		require.NoError(t, err)
		assert.Equal(t, `{"burstable":false}`, pi.PreviewAnnotation())
	})

	t.Run("UpdateFromProfile removes preview annotation when dropped", func(t *testing.T) {
		profile := minimalProfile("p1")
		profile.Annotations = map[string]string{PreviewAnnotationKey: `{"burstable":true}`}
		pi, err := NewPodAutoscalerProfileInternal(profile)
		require.NoError(t, err)
		assert.Equal(t, `{"burstable":true}`, pi.PreviewAnnotation())

		// Simulate annotation removal
		profile.Annotations = nil
		err = pi.UpdateFromProfile(profile)
		require.NoError(t, err)
		assert.Empty(t, pi.PreviewAnnotation())
	})
}

func TestGenerateDPAName(t *testing.T) {
	name := generateDPAName(NamespacedObjectReference{GroupKind: schema.GroupKind{Group: "apps", Kind: "Deployment"}, Namespace: "prod", Name: "web-app"})
	assert.Equal(t, "web-app-9526aeb3", name)

	// Deterministic
	name2 := generateDPAName(NamespacedObjectReference{GroupKind: schema.GroupKind{Group: "apps", Kind: "Deployment"}, Namespace: "prod", Name: "web-app"})
	assert.Equal(t, name, name2)

	// Different kind produces different name
	nameSTF := generateDPAName(NamespacedObjectReference{GroupKind: schema.GroupKind{Group: "apps", Kind: "StatefulSet"}, Namespace: "prod", Name: "web-app"})
	assert.Equal(t, "web-app-c3b1042a", nameSTF)
}
