// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package libraryinjection_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
)

func TestGetProviderForPod(t *testing.T) {
	// Mock the CSI driver as available and SSI-enabled for tests that expect CSIProvider
	cleanup := libraryinjection.SetCSIStatusFetcherForTest(func(_ context.Context) libraryinjection.CSIDriverStatus {
		return libraryinjection.CSIDriverStatus{
			Available:  true,
			Version:    "1.0.0",
			SSIEnabled: true,
		}
	})
	defer cleanup()

	tests := []struct {
		name             string
		defaultMode      libraryinjection.InjectionMode
		csiEnabled       bool
		annotationValue  *string // nil = no annotation
		expectedProvider string
	}{
		{
			name:             "no annotation uses default init_container",
			defaultMode:      libraryinjection.InjectionModeInitContainer,
			csiEnabled:       true,
			annotationValue:  nil,
			expectedProvider: "InitContainerProvider",
		},
		{
			name:             "no annotation uses default csi",
			defaultMode:      libraryinjection.InjectionModeCSI,
			csiEnabled:       true,
			annotationValue:  nil,
			expectedProvider: "CSIProvider",
		},
		{
			name:             "no annotation uses default auto",
			defaultMode:      libraryinjection.InjectionModeAuto,
			csiEnabled:       true,
			annotationValue:  nil,
			expectedProvider: "AutoProvider",
		},
		{
			name:             "annotation overrides default: init_container -> csi",
			defaultMode:      libraryinjection.InjectionModeInitContainer,
			csiEnabled:       true,
			annotationValue:  ptr.To("csi"),
			expectedProvider: "CSIProvider",
		},
		{
			name:             "annotation overrides default: csi -> init_container",
			defaultMode:      libraryinjection.InjectionModeCSI,
			csiEnabled:       true,
			annotationValue:  ptr.To("init_container"),
			expectedProvider: "InitContainerProvider",
		},
		{
			name:             "annotation auto uses AutoProvider",
			defaultMode:      libraryinjection.InjectionModeInitContainer,
			csiEnabled:       true,
			annotationValue:  ptr.To("auto"),
			expectedProvider: "AutoProvider",
		},
		{
			name:             "unknown mode falls back to auto",
			defaultMode:      libraryinjection.InjectionModeCSI,
			csiEnabled:       true,
			annotationValue:  ptr.To("unknown_mode"),
			expectedProvider: "AutoProvider",
		},
		{
			name:             "empty annotation uses default",
			defaultMode:      libraryinjection.InjectionModeCSI,
			csiEnabled:       true,
			annotationValue:  ptr.To(""),
			expectedProvider: "CSIProvider",
		},
		{
			name:             "csi disabled in config falls back to init_container",
			defaultMode:      libraryinjection.InjectionModeCSI,
			csiEnabled:       false,
			annotationValue:  nil,
			expectedProvider: "InitContainerProvider",
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

			factory := libraryinjection.NewProviderFactory(tt.defaultMode, tt.csiEnabled)
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

func TestGetProviderForPod_CSIFallback(t *testing.T) {
	tests := []struct {
		name             string
		csiStatus        libraryinjection.CSIDriverStatus
		expectedProvider string
	}{
		{
			name: "CSI driver not available falls back to init_container",
			csiStatus: libraryinjection.CSIDriverStatus{
				Available:  false,
				SSIEnabled: false,
			},
			expectedProvider: "InitContainerProvider",
		},
		{
			name: "CSI driver available but SSI not enabled falls back to init_container",
			csiStatus: libraryinjection.CSIDriverStatus{
				Available:  true,
				Version:    "1.0.0",
				SSIEnabled: false,
			},
			expectedProvider: "InitContainerProvider",
		},
		{
			name: "CSI driver available and SSI enabled uses CSI",
			csiStatus: libraryinjection.CSIDriverStatus{
				Available:  true,
				Version:    "1.0.0",
				SSIEnabled: true,
			},
			expectedProvider: "CSIProvider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleanup := libraryinjection.SetCSIStatusFetcherForTest(func(_ context.Context) libraryinjection.CSIDriverStatus {
				return tt.csiStatus
			})
			defer cleanup()

			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Annotations: map[string]string{
						annotation.InjectionMode: "csi",
					},
				},
			}

			factory := libraryinjection.NewProviderFactory(libraryinjection.InjectionModeCSI, true) // CSI enabled in config
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
