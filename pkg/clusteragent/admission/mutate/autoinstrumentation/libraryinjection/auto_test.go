// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package libraryinjection_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
)

const datadogCSIDriverName = "k8s.csi.datadoghq.com"

var defaultCSIAutoRegistries = []string{"gcr.io/datadoghq", "public.ecr.aws/datadog"}

// fakeCSIDriverWatcher is a deterministic CSIDriverWatcher implementation
// used in unit tests so we can drive the AutoProvider decision tree without
// spinning up a workloadmeta subscription.
type fakeCSIDriverWatcher struct {
	registered bool
	apmEnabled bool
}

func (f fakeCSIDriverWatcher) IsRegistered() bool { return f.registered }
func (f fakeCSIDriverWatcher) IsAPMEnabled() bool { return f.registered && f.apmEnabled }

func newPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "my-app:latest"}},
		},
	}
}

func injectorConfig() libraryinjection.InjectorConfig {
	return libraryinjection.InjectorConfig{
		Package: libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/apm-inject:0.52.0", "0.52.0"),
	}
}

// findInstrumentationVolume returns the pod volume named
// libraryinjection.InstrumentationVolumeName, failing the test if absent.
func findInstrumentationVolume(t *testing.T, pod *corev1.Pod) *corev1.Volume {
	t.Helper()
	for i := range pod.Spec.Volumes {
		if pod.Spec.Volumes[i].Name == libraryinjection.InstrumentationVolumeName {
			return &pod.Spec.Volumes[i]
		}
	}
	t.Fatalf("instrumentation volume %q not found", libraryinjection.InstrumentationVolumeName)
	return nil
}

func TestAutoProvider_PicksCSIWhenWatcherReportsAPMEnabled(t *testing.T) {
	pod := newPod()

	provider := libraryinjection.NewAutoProvider(libraryinjection.LibraryInjectionConfig{
		Injector:          injectorConfig(),
		CSIAutoRegistries: defaultCSIAutoRegistries,
		CSIDriverWatcher:  fakeCSIDriverWatcher{registered: true, apmEnabled: true},
	})

	result := provider.InjectInjector(pod, injectorConfig())
	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	// CSIProvider injects the InstrumentationVolume as a CSI volume; the
	// init-container provider would have used an EmptyDir instead. Use that
	// difference as the discriminating signal between the two strategies.
	r := require.New(t)
	vol := findInstrumentationVolume(t, pod)
	r.NotNil(vol.CSI, "instrumentation volume should be a CSI volume")
	r.Equal(datadogCSIDriverName, vol.CSI.Driver)
}

func TestAutoProvider_RegistrySelection(t *testing.T) {
	tests := []struct {
		name     string
		cfg      libraryinjection.LibraryInjectionConfig
		wantMode string
	}{
		{
			name: "selects csi when every image uses an allowed registry",
			cfg: libraryinjection.LibraryInjectionConfig{
				Injector: injectorConfig(),
				Libraries: []libraryinjection.LibraryConfig{
					{Package: libraryinjection.LibraryImage{Registry: "public.ecr.aws/datadog"}},
				},
				CSIAutoRegistries: []string{"gcr.io/datadoghq", "public.ecr.aws/datadog"},
			},
			wantMode: "csi (auto)",
		},
		{
			name: "falls back when the injector uses another registry",
			cfg: libraryinjection.LibraryInjectionConfig{
				Injector: libraryinjection.InjectorConfig{
					Package: libraryinjection.LibraryImage{Registry: "registry.example.com/datadog"},
				},
				CSIAutoRegistries: []string{"gcr.io/datadoghq"},
			},
			wantMode: "init_container (auto)",
		},
		{
			name: "falls back when a library uses another registry",
			cfg: libraryinjection.LibraryInjectionConfig{
				Injector: injectorConfig(),
				Libraries: []libraryinjection.LibraryConfig{
					{Package: libraryinjection.LibraryImage{Registry: "registry.example.com/datadog"}},
				},
				CSIAutoRegistries: []string{"gcr.io/datadoghq"},
			},
			wantMode: "init_container (auto)",
		},
		{
			name: "falls back when no csi registry is configured",
			cfg: libraryinjection.LibraryInjectionConfig{
				Injector: injectorConfig(),
			},
			wantMode: "init_container (auto)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.cfg.CSIDriverWatcher = fakeCSIDriverWatcher{registered: true, apmEnabled: true}
			require.Equal(t, tt.wantMode, libraryinjection.NewAutoProvider(tt.cfg).GetName())
		})
	}
}

func TestAutoProvider_FallsBackToInitContainerWhenWatcherReportsAPMDisabled(t *testing.T) {
	// The watcher knows about the CSI driver but APM is not advertised on it
	// (annotation missing, or set to anything other than "true"). AutoProvider
	// must stay on the safe init-container path.
	pod := newPod()

	provider := libraryinjection.NewAutoProvider(libraryinjection.LibraryInjectionConfig{
		CSIDriverWatcher: fakeCSIDriverWatcher{registered: true, apmEnabled: false},
	})

	result := provider.InjectInjector(pod, injectorConfig())
	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	vol := findInstrumentationVolume(t, pod)
	assert.Nil(t, vol.CSI, "instrumentation volume must not be a CSI volume when APM is not advertised")
	assert.NotNil(t, vol.EmptyDir, "instrumentation volume must be an EmptyDir when APM is not advertised")
}

func TestAutoProvider_FallsBackToInitContainerWhenWatcherIsNil(t *testing.T) {
	// A nil watcher means CSI auto-detection is disabled (e.g. the temporary
	// feature flag is off, or the cluster-agent runs without workloadmeta).
	// AutoProvider must behave exactly as before this feature existed.
	pod := newPod()

	provider := libraryinjection.NewAutoProvider(libraryinjection.LibraryInjectionConfig{
		CSIDriverWatcher: nil,
	})

	result := provider.InjectInjector(pod, injectorConfig())
	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	vol := findInstrumentationVolume(t, pod)
	assert.Nil(t, vol.CSI, "with nil watcher, AutoProvider must not produce CSI volumes")
	assert.NotNil(t, vol.EmptyDir, "with nil watcher, AutoProvider must fall back to an EmptyDir volume")
}
