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
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
)

func TestGetProviderForPod(t *testing.T) {
	tests := []struct {
		name             string
		defaultMode      libraryinjection.InjectionMode
		annotationValue  *string // nil = no annotation
		expectedProvider string
	}{
		{
			name:             "no annotation uses default init_container",
			defaultMode:      libraryinjection.InjectionModeInitContainer,
			annotationValue:  nil,
			expectedProvider: "InitContainerProvider",
		},
		{
			name:             "no annotation uses default csi",
			defaultMode:      libraryinjection.InjectionModeCSI,
			annotationValue:  nil,
			expectedProvider: "CSIProvider",
		},
		{
			name:             "no annotation uses default auto",
			defaultMode:      libraryinjection.InjectionModeAuto,
			annotationValue:  nil,
			expectedProvider: "AutoProvider",
		},
		{
			name:             "annotation overrides default: init_container -> csi",
			defaultMode:      libraryinjection.InjectionModeInitContainer,
			annotationValue:  ptr.To("csi"),
			expectedProvider: "CSIProvider",
		},
		{
			name:             "annotation overrides default: csi -> init_container",
			defaultMode:      libraryinjection.InjectionModeCSI,
			annotationValue:  ptr.To("init_container"),
			expectedProvider: "InitContainerProvider",
		},
		{
			name:             "annotation auto uses AutoProvider",
			defaultMode:      libraryinjection.InjectionModeInitContainer,
			annotationValue:  ptr.To("auto"),
			expectedProvider: "AutoProvider",
		},
		{
			name:             "unknown mode falls back to auto",
			defaultMode:      libraryinjection.InjectionModeCSI,
			annotationValue:  ptr.To("unknown_mode"),
			expectedProvider: "AutoProvider",
		},
		{
			name:             "empty annotation uses default",
			defaultMode:      libraryinjection.InjectionModeCSI,
			annotationValue:  ptr.To(""),
			expectedProvider: "CSIProvider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
			}

			if tt.annotationValue != nil {
				pod.Annotations = map[string]string{
					annotation.InjectionMode: *tt.annotationValue,
				}
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
			case "AutoProvider":
				_, ok := provider.(*libraryinjection.AutoProvider)
				assert.True(t, ok, "expected AutoProvider")
			}
		})
	}
}

func TestGetProviderForPod_ImageVolume_Compatibility(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Annotations: map[string]string{
				annotation.InjectionMode: "image_volume",
			},
		},
	}

	factory := libraryinjection.NewProviderFactory(libraryinjection.InjectionModeAuto)

	// No server version info -> stop injection.
	provider := factory.GetProviderForPod(pod, libraryinjection.LibraryInjectionConfig{})
	_, ok := provider.(*libraryinjection.NoopProvider)
	assert.True(t, ok, "expected NoopProvider when server version is unknown")

	// Too old -> stop injection.
	provider = factory.GetProviderForPod(pod, libraryinjection.LibraryInjectionConfig{
		KubeServerVersion: &version.Info{GitVersion: "v1.30.9"},
	})
	_, ok = provider.(*libraryinjection.NoopProvider)
	assert.True(t, ok, "expected NoopProvider when server version is too old")

	// Supported -> image volume provider.
	provider = factory.GetProviderForPod(pod, libraryinjection.LibraryInjectionConfig{
		KubeServerVersion: &version.Info{GitVersion: "v1.33.0"},
	})
	_, ok = provider.(*libraryinjection.ImageVolumeProvider)
	assert.True(t, ok, "expected ImageVolumeProvider when server version supports image volumes")
}
