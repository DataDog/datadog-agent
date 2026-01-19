// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package libraryinjection_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
)

func TestGetProviderForPod_DefaultMode(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
	}

	// Default mode is init_container
	factory := libraryinjection.NewProviderFactory(libraryinjection.InjectionModeInitContainer)
	provider := factory.GetProviderForPod(pod, libraryinjection.LibraryInjectionConfig{})

	// Should return InitContainerProvider
	_, isInitContainer := provider.(*libraryinjection.InitContainerProvider)
	assert.True(t, isInitContainer, "expected InitContainerProvider")
}

func TestGetProviderForPod_AnnotationOverridesDefault(t *testing.T) {
	tests := []struct {
		name             string
		defaultMode      libraryinjection.InjectionMode
		annotationValue  string
		expectedProvider string
	}{
		{
			name:             "default init_container, annotation csi",
			defaultMode:      libraryinjection.InjectionModeInitContainer,
			annotationValue:  "csi",
			expectedProvider: "CSIProvider",
		},
		{
			name:             "default csi, annotation init_container",
			defaultMode:      libraryinjection.InjectionModeCSI,
			annotationValue:  "init_container",
			expectedProvider: "InitContainerProvider",
		},
		{
			name:             "default init_container, annotation auto",
			defaultMode:      libraryinjection.InjectionModeInitContainer,
			annotationValue:  "auto",
			expectedProvider: "InitContainerProvider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Annotations: map[string]string{
						annotation.InjectionMode: tt.annotationValue,
					},
				},
			}

			factory := libraryinjection.NewProviderFactory(tt.defaultMode)
			provider := factory.GetProviderForPod(pod, libraryinjection.LibraryInjectionConfig{})

			switch tt.expectedProvider {
			case "CSIProvider":
				_, ok := provider.(*libraryinjection.CSIProvider)
				assert.True(t, ok, "expected CSIProvider")
			case "InitContainerProvider":
				_, ok := provider.(*libraryinjection.InitContainerProvider)
				assert.True(t, ok, "expected InitContainerProvider")
			}
		})
	}
}

func TestGetProviderForPod_UnknownModeFallsBackToAuto(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Annotations: map[string]string{
				annotation.InjectionMode: "unknown_mode",
			},
		},
	}

	factory := libraryinjection.NewProviderFactory(libraryinjection.InjectionModeCSI)
	provider := factory.GetProviderForPod(pod, libraryinjection.LibraryInjectionConfig{})

	// Unknown mode should fallback to InitContainerProvider (auto behavior)
	_, isInitContainer := provider.(*libraryinjection.InitContainerProvider)
	assert.True(t, isInitContainer, "unknown mode should fallback to InitContainerProvider")
}

func TestGetProviderForPod_EmptyAnnotationUsesDefault(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Annotations: map[string]string{
				annotation.InjectionMode: "",
			},
		},
	}

	// Default is CSI
	factory := libraryinjection.NewProviderFactory(libraryinjection.InjectionModeCSI)
	provider := factory.GetProviderForPod(pod, libraryinjection.LibraryInjectionConfig{})

	// Empty annotation should use default (CSI)
	_, isCSI := provider.(*libraryinjection.CSIProvider)
	assert.True(t, isCSI, "empty annotation should use default mode")
}
