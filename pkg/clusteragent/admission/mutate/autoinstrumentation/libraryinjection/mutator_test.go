// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package libraryinjection_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
)

// libraryAnnotationEntry mirrors the unexported injectedLibraryEntry struct
// for assertion purposes in tests.
type libraryAnnotationEntry struct {
	Name   string `json:"name"`
	Image  string `json:"image"`
	Status string `json:"status"`
}

func parseInjectedLibraries(t *testing.T, pod *corev1.Pod) []libraryAnnotationEntry {
	t.Helper()
	val, ok := annotation.Get(pod, annotation.InjectedLibraries)
	require.True(t, ok, "InjectedLibraries annotation should be set")
	var entries []libraryAnnotationEntry
	require.NoError(t, json.Unmarshal([]byte(val), &entries))
	return entries
}

func javaLib() libraryinjection.LibraryConfig {
	return libraryinjection.LibraryConfig{
		Language: "java",
		Package:  libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/dd-lib-java-init:1.30.0", "1.30.0"),
	}
}

// TestGetName verifies that each provider reports its effective injection mode correctly.
func TestGetName(t *testing.T) {
	tests := []struct {
		name     string
		provider libraryinjection.LibraryInjectionProvider
		expected string
	}{
		{
			name:     "InitContainerProvider",
			provider: libraryinjection.NewInitContainerProvider(libraryinjection.LibraryInjectionConfig{}),
			expected: "init_container",
		},
		{
			name:     "CSIProvider",
			provider: libraryinjection.NewCSIProvider(libraryinjection.LibraryInjectionConfig{}),
			expected: "csi",
		},
		{
			name:     "ImageVolumeProvider",
			provider: libraryinjection.NewImageVolumeProvider(libraryinjection.LibraryInjectionConfig{}),
			expected: "image_volume",
		},
		{
			name: "AutoProvider resolves to CSI when driver is available",
			provider: libraryinjection.NewAutoProvider(libraryinjection.LibraryInjectionConfig{
				CSIDriverWatcher: fakeCSIDriverWatcher{registered: true, apmEnabled: true},
			}),
			expected: "csi (auto)",
		},
		{
			name: "AutoProvider resolves to init_container when driver is unavailable",
			provider: libraryinjection.NewAutoProvider(libraryinjection.LibraryInjectionConfig{
				CSIDriverWatcher: fakeCSIDriverWatcher{registered: true, apmEnabled: false},
			}),
			expected: "init_container (auto)",
		},
		{
			name: "AutoProvider resolves to init_container when watcher is nil",
			provider: libraryinjection.NewAutoProvider(libraryinjection.LibraryInjectionConfig{
				CSIDriverWatcher: nil,
			}),
			expected: "init_container (auto)",
		},
		{
			name: "NoopProvider returns disabled",
			provider: func() libraryinjection.LibraryInjectionProvider {
				factory := libraryinjection.NewProviderFactory(libraryinjection.InjectionModeAuto)
				pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
					annotation.InjectionMode: "image_volume",
				}}}
				return factory.GetProviderForPod(pod, libraryinjection.LibraryInjectionConfig{
					KubeServerVersion: &version.Info{GitVersion: "v1.30.9"},
				})
			}(),
			expected: "disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.provider.GetName())
		})
	}
}

// TestInjectAPMLibraries_Annotations_InitContainer verifies the annotations
// written for a successful init_container injection.
func TestInjectAPMLibraries_Annotations_InitContainer(t *testing.T) {
	pod := newPod()

	err := libraryinjection.InjectAPMLibraries(pod, libraryinjection.LibraryInjectionConfig{
		InjectionMode: string(libraryinjection.InjectionModeInitContainer),
		Injector:      injectorConfig(),
		Libraries:     []libraryinjection.LibraryConfig{javaLib()},
	})
	require.NoError(t, err)

	mode, ok := annotation.Get(pod, annotation.EffectiveInjectionMode)
	require.True(t, ok)
	assert.Equal(t, "init_container", mode)

	status, ok := annotation.Get(pod, annotation.InjectionStatus)
	require.True(t, ok)
	assert.Equal(t, annotation.InjectionStatusInjected, status)

	entries := parseInjectedLibraries(t, pod)
	require.Len(t, entries, 2)
	assert.Equal(t, libraryAnnotationEntry{Name: "injector", Image: "gcr.io/datadoghq/apm-inject:0.52.0", Status: "injected"}, entries[0])
	assert.Equal(t, libraryAnnotationEntry{Name: "java", Image: "gcr.io/datadoghq/dd-lib-java-init:1.30.0", Status: "injected"}, entries[1])
}

// TestInjectAPMLibraries_Annotations_CSI verifies the annotations written
// for a successful CSI injection.
func TestInjectAPMLibraries_Annotations_CSI(t *testing.T) {
	pod := newPod()

	err := libraryinjection.InjectAPMLibraries(pod, libraryinjection.LibraryInjectionConfig{
		InjectionMode: string(libraryinjection.InjectionModeCSI),
		Injector:      injectorConfig(),
		Libraries:     []libraryinjection.LibraryConfig{javaLib()},
	})
	require.NoError(t, err)

	mode, ok := annotation.Get(pod, annotation.EffectiveInjectionMode)
	require.True(t, ok)
	assert.Equal(t, "csi", mode)

	entries := parseInjectedLibraries(t, pod)
	require.Len(t, entries, 2)
	assert.Equal(t, libraryAnnotationEntry{Name: "injector", Image: "gcr.io/datadoghq/apm-inject:0.52.0", Status: "injected"}, entries[0])
	assert.Equal(t, libraryAnnotationEntry{Name: "java", Image: "gcr.io/datadoghq/dd-lib-java-init:1.30.0", Status: "injected"}, entries[1])
}

// TestInjectAPMLibraries_Annotations_Auto_CSI verifies that auto mode resolving
// to CSI reports "csi (auto)" as the effective injection mode.
func TestInjectAPMLibraries_Annotations_Auto_CSI(t *testing.T) {
	pod := newPod()

	err := libraryinjection.InjectAPMLibraries(pod, libraryinjection.LibraryInjectionConfig{
		InjectionMode:    string(libraryinjection.InjectionModeAuto),
		CSIDriverWatcher: fakeCSIDriverWatcher{registered: true, apmEnabled: true},
		Injector:         injectorConfig(),
		Libraries:        []libraryinjection.LibraryConfig{javaLib()},
	})
	require.NoError(t, err)

	mode, ok := annotation.Get(pod, annotation.EffectiveInjectionMode)
	require.True(t, ok)
	assert.Equal(t, "csi (auto)", mode)
}

// TestInjectAPMLibraries_Annotations_Auto_InitContainer verifies that auto mode
// falling back to init_container reports "init_container (auto)".
func TestInjectAPMLibraries_Annotations_Auto_InitContainer(t *testing.T) {
	pod := newPod()

	err := libraryinjection.InjectAPMLibraries(pod, libraryinjection.LibraryInjectionConfig{
		InjectionMode:    string(libraryinjection.InjectionModeAuto),
		CSIDriverWatcher: fakeCSIDriverWatcher{registered: true, apmEnabled: false},
		Injector:         injectorConfig(),
		Libraries:        []libraryinjection.LibraryConfig{javaLib()},
	})
	require.NoError(t, err)

	mode, ok := annotation.Get(pod, annotation.EffectiveInjectionMode)
	require.True(t, ok)
	assert.Equal(t, "init_container (auto)", mode)
}

// TestInjectAPMLibraries_Annotations_Skipped verifies that when injection is
// skipped (e.g. incompatible k8s version), EffectiveInjectionMode is still set
// but InjectedLibraries is not, and InjectionStatus is "skipped".
func TestInjectAPMLibraries_Annotations_Skipped(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Annotations: map[string]string{
				annotation.InjectionMode: "image_volume",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app"}},
		},
	}

	err := libraryinjection.InjectAPMLibraries(pod, libraryinjection.LibraryInjectionConfig{
		InjectionMode:     "auto",
		KubeServerVersion: &version.Info{GitVersion: "v1.30.9"},
		Injector:          injectorConfig(),
	})
	require.NoError(t, err)

	// EffectiveInjectionMode is set immediately after provider selection, even on skip.
	_, ok := annotation.Get(pod, annotation.EffectiveInjectionMode)
	assert.True(t, ok, "EffectiveInjectionMode should be set even when injection is skipped")

	// InjectedLibraries must NOT be set: nothing was actually injected.
	_, ok = annotation.Get(pod, annotation.InjectedLibraries)
	assert.False(t, ok, "InjectedLibraries should not be set when injection is skipped")

	// InjectionStatus must be "skipped".
	status, ok := annotation.Get(pod, annotation.InjectionStatus)
	require.True(t, ok, "InjectionStatus should be set when injection is skipped")
	assert.Equal(t, annotation.InjectionStatusSkipped, status)

	// The existing error annotation must still be present.
	val, ok := annotation.Get(pod, annotation.InjectionError)
	require.True(t, ok)
	require.Contains(t, val, "requires kubernetes version")
}

// TestInjectAPMLibraries_InjectedLibraries_UnsupportedLanguage verifies that
// an unsupported language gets a "skipped" status in the InjectedLibraries annotation
// and that the global InjectionStatus is "partial".
func TestInjectAPMLibraries_InjectedLibraries_UnsupportedLanguage(t *testing.T) {
	pod := newPod()

	err := libraryinjection.InjectAPMLibraries(pod, libraryinjection.LibraryInjectionConfig{
		InjectionMode: string(libraryinjection.InjectionModeCSI),
		Injector:      injectorConfig(),
		Libraries: []libraryinjection.LibraryConfig{
			javaLib(),
			{
				Language: "cobol",
				Package:  libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/dd-lib-cobol-init:1.0.0", "1.0.0"),
			},
		},
	})
	// InjectAPMLibraries still reports per-library failures via the returned error.
	// It is the caller's responsibility (apmInjectionMutator) to absorb it so the
	// patch is applied. Here we verify the error is present and the annotations are set.
	require.Error(t, err)

	status, ok := annotation.Get(pod, annotation.InjectionStatus)
	require.True(t, ok)
	assert.Equal(t, annotation.InjectionStatusPartial, status)

	// The injection-error annotation must carry the human-readable reason.
	injErr, ok := annotation.Get(pod, annotation.InjectionError)
	require.True(t, ok, "InjectionError annotation should be set for partial injection")
	assert.Contains(t, injErr, "cobol")

	entries := parseInjectedLibraries(t, pod)
	require.Len(t, entries, 3)
	assert.Equal(t, libraryAnnotationEntry{Name: "injector", Image: "gcr.io/datadoghq/apm-inject:0.52.0", Status: "injected"}, entries[0])
	assert.Equal(t, libraryAnnotationEntry{Name: "java", Image: "gcr.io/datadoghq/dd-lib-java-init:1.30.0", Status: "injected"}, entries[1])
	assert.Equal(t, libraryAnnotationEntry{Name: "cobol", Image: "gcr.io/datadoghq/dd-lib-cobol-init:1.0.0", Status: "skipped"}, entries[2])
}

// TestInjectAPMLibraries_InjectionStatus verifies the global InjectionStatus annotation
// across the full range of outcomes.
func TestInjectAPMLibraries_InjectionStatus(t *testing.T) {
	tests := []struct {
		name          string
		cfg           libraryinjection.LibraryInjectionConfig
		pod           *corev1.Pod
		wantErr       bool
		wantStatus    string
		wantStatusSet bool
	}{
		{
			name: "all libraries injected → injected",
			pod:  newPod(),
			cfg: libraryinjection.LibraryInjectionConfig{
				InjectionMode: string(libraryinjection.InjectionModeCSI),
				Injector:      injectorConfig(),
				Libraries:     []libraryinjection.LibraryConfig{javaLib()},
			},
			wantStatus:    annotation.InjectionStatusInjected,
			wantStatusSet: true,
		},
		{
			name: "no libraries configured → injected (injector OK, nothing else requested)",
			pod:  newPod(),
			cfg: libraryinjection.LibraryInjectionConfig{
				InjectionMode: string(libraryinjection.InjectionModeCSI),
				Injector:      injectorConfig(),
				Libraries:     nil,
			},
			wantStatus:    annotation.InjectionStatusInjected,
			wantStatusSet: true,
		},
		{
			name: "only unsupported languages → partial",
			pod:  newPod(),
			cfg: libraryinjection.LibraryInjectionConfig{
				InjectionMode: string(libraryinjection.InjectionModeCSI),
				Injector:      injectorConfig(),
				Libraries: []libraryinjection.LibraryConfig{
					{
						Language: "cobol",
						Package:  libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/dd-lib-cobol-init:1.0.0", "1.0.0"),
					},
				},
			},
			// InjectAPMLibraries reports the per-library failure via the error return.
			// The caller (apmInjectionMutator) absorbs it so the patch is still applied.
			wantErr:       true,
			wantStatus:    annotation.InjectionStatusPartial,
			wantStatusSet: true,
		},
		{
			name: "injector skipped (incompatible k8s version) → skipped",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
					Annotations: map[string]string{
						annotation.InjectionMode: string(libraryinjection.InjectionModeImageVolume),
					},
				},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app"}}},
			},
			cfg: libraryinjection.LibraryInjectionConfig{
				InjectionMode:     string(libraryinjection.InjectionModeAuto),
				KubeServerVersion: &version.Info{GitVersion: "v1.30.9"},
				Injector:          injectorConfig(),
				Libraries:         []libraryinjection.LibraryConfig{javaLib()},
			},
			wantStatus:    annotation.InjectionStatusSkipped,
			wantStatusSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := libraryinjection.InjectAPMLibraries(tt.pod, tt.cfg)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			status, ok := annotation.Get(tt.pod, annotation.InjectionStatus)
			require.Equal(t, tt.wantStatusSet, ok, "InjectionStatus annotation presence mismatch")
			if tt.wantStatusSet {
				assert.Equal(t, tt.wantStatus, status)
			}
		})
	}
}

// TestInjectAPMLibraries_CSIDriverStatus verifies that the csi-driver-status annotation
// reflects the three possible states of the Datadog CSI driver.
func TestInjectAPMLibraries_CSIDriverStatus(t *testing.T) {
	tests := []struct {
		name           string
		watcher        libraryinjection.CSIDriverWatcher
		wantStatus     string
		wantAnnotation bool
	}{
		{
			name:           "driver installed and APM enabled → apm-enabled",
			watcher:        fakeCSIDriverWatcher{registered: true, apmEnabled: true},
			wantStatus:     annotation.CSIDriverStatusAPMEnabled,
			wantAnnotation: true,
		},
		{
			name:           "driver installed but APM disabled → apm-disabled",
			watcher:        fakeCSIDriverWatcher{registered: true, apmEnabled: false},
			wantStatus:     annotation.CSIDriverStatusAPMDisabled,
			wantAnnotation: true,
		},
		{
			name:           "driver not installed → not-installed",
			watcher:        fakeCSIDriverWatcher{registered: false, apmEnabled: false},
			wantStatus:     annotation.CSIDriverStatusNotInstalled,
			wantAnnotation: true,
		},
		{
			name:           "watcher nil (CSI detection disabled) → annotation absent",
			watcher:        nil,
			wantAnnotation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := newPod()
			err := libraryinjection.InjectAPMLibraries(pod, libraryinjection.LibraryInjectionConfig{
				InjectionMode:    string(libraryinjection.InjectionModeCSI),
				CSIDriverWatcher: tt.watcher,
				Injector:         injectorConfig(),
				Libraries:        []libraryinjection.LibraryConfig{javaLib()},
			})
			require.NoError(t, err)

			status, ok := annotation.Get(pod, annotation.CSIDriverStatus)
			require.Equal(t, tt.wantAnnotation, ok, "CSIDriverStatus annotation presence mismatch")
			if tt.wantAnnotation {
				assert.Equal(t, tt.wantStatus, status)
			}
		})
	}
}

func TestInjectAPMLibraries_StopsGracefullyWhenProviderUnavailable(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Annotations: map[string]string{
				annotation.InjectionMode: "image_volume",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app"}},
		},
	}

	err := libraryinjection.InjectAPMLibraries(pod, libraryinjection.LibraryInjectionConfig{
		InjectionMode:     "auto",
		KubeServerVersion: &version.Info{GitVersion: "v1.30.9"},
		Injector: libraryinjection.InjectorConfig{
			Package: libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/apm-inject:0.52.0", "0.52.0"),
		},
	})
	require.NoError(t, err)

	val, ok := annotation.Get(pod, annotation.InjectionError)
	require.True(t, ok)
	require.Contains(t, val, "requires kubernetes version")
}
