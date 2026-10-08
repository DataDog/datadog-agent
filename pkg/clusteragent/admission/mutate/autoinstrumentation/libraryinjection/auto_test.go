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
	"k8s.io/apimachinery/pkg/version"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
)

const datadogCSIDriverName = "k8s.csi.datadoghq.com"

var defaultCSIAutoRegistries = []string{"gcr.io/datadoghq", "public.ecr.aws/datadog"}

var csiKubeVersion = &version.Info{GitVersion: "v1.30.9"}

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
		KubeServerVersion: csiKubeVersion,
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
			tt.cfg.KubeServerVersion = csiKubeVersion
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
	// A nil watcher means CSI auto-detection is unavailable (e.g. the
	// cluster-agent runs without workloadmeta).
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

func TestAutoProvider_FallsBackToInitContainerOnOpenShift(t *testing.T) {
	pod := newPod()

	provider := libraryinjection.NewAutoProvider(libraryinjection.LibraryInjectionConfig{
		Injector:          injectorConfig(),
		CSIAutoRegistries: defaultCSIAutoRegistries,
		CSIDriverWatcher:  fakeCSIDriverWatcher{registered: true, apmEnabled: true},
		KubeServerVersion: csiKubeVersion,
		IsOpenShift:       true,
	})

	result := provider.InjectInjector(pod, injectorConfig())
	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	vol := findInstrumentationVolume(t, pod)
	assert.Nil(t, vol.CSI, "on OpenShift, AutoProvider must not produce CSI volumes")
	assert.NotNil(t, vol.EmptyDir, "on OpenShift, AutoProvider must fall back to an EmptyDir volume")
}

func TestAutoProvider_LibraryVersion(t *testing.T) {
	lib := func(language, version, canonicalVersion string) libraryinjection.LibraryConfig {
		return libraryinjection.LibraryConfig{
			Language: language,
			Package: libraryinjection.LibraryImage{
				Registry:         "gcr.io/datadoghq",
				Name:             "dd-lib-" + language + "-init",
				Version:          version,
				CanonicalVersion: canonicalVersion,
			},
		}
	}
	tests := []struct {
		name      string
		libraries []libraryinjection.LibraryConfig
		wantMode  string
	}{
		{"uses csi with the default java version", []libraryinjection.LibraryConfig{lib("java", "v1", "")}, "csi (auto)"},
		{"uses csi with the default python version", []libraryinjection.LibraryConfig{lib("python", "v4", "")}, "csi (auto)"},
		{"uses csi with the default js version", []libraryinjection.LibraryConfig{lib("js", "v6", "")}, "csi (auto)"},
		{"uses csi with the default dotnet version", []libraryinjection.LibraryConfig{lib("dotnet", "v3", "")}, "csi (auto)"},
		{"uses csi with the default ruby version", []libraryinjection.LibraryConfig{lib("ruby", "v2", "")}, "csi (auto)"},
		{"uses csi with the default php version", []libraryinjection.LibraryConfig{lib("php", "v1", "")}, "csi (auto)"},
		{"uses csi with latest", []libraryinjection.LibraryConfig{lib("python", "latest", "")}, "csi (auto)"},
		{"uses csi with the first python version with the layout", []libraryinjection.LibraryConfig{lib("python", "v2.11.0", "")}, "csi (auto)"},
		{"uses csi with a python minor tag with the layout", []libraryinjection.LibraryConfig{lib("python", "v2.11", "")}, "csi (auto)"},
		{"uses csi with a python version without v prefix", []libraryinjection.LibraryConfig{lib("python", "2.12.0", "")}, "csi (auto)"},
		{"uses csi with the js v4 major tag", []libraryinjection.LibraryConfig{lib("js", "v4", "")}, "csi (auto)"},
		{"uses csi with the js v5 major tag", []libraryinjection.LibraryConfig{lib("js", "v5", "")}, "csi (auto)"},
		{"uses csi with a js v4 minor tag with the layout", []libraryinjection.LibraryConfig{lib("js", "v4.45", "")}, "csi (auto)"},
		{"uses csi with the first js v5 version with the layout", []libraryinjection.LibraryConfig{lib("js", "v5.21.0", "")}, "csi (auto)"},
		{"uses csi with any php version", []libraryinjection.LibraryConfig{lib("php", "v0.1.0", "")}, "csi (auto)"},
		{"uses csi with a digest and a recent canonical version", []libraryinjection.LibraryConfig{lib("java", "sha256:abc", "v1.40.0")}, "csi (auto)"},
		{"uses csi with a digest", []libraryinjection.LibraryConfig{lib("java", "sha256:abc", "")}, "csi (auto)"},
		{"uses csi with a custom tag", []libraryinjection.LibraryConfig{lib("ruby", "custom", "")}, "csi (auto)"},
		{"uses csi with a dev build tag", []libraryinjection.LibraryConfig{lib("ruby", "2.4.0.dev.ba4d451", "")}, "csi (auto)"},
		{"uses csi with a recent prerelease", []libraryinjection.LibraryConfig{lib("js", "4.47.0-pipeline.45492227.beta.b42c17c4", "")}, "csi (auto)"},
		{"falls back with an old python version", []libraryinjection.LibraryConfig{lib("python", "v2.10.4", "")}, "init_container (auto)"},
		{"falls back with an old python version without v prefix", []libraryinjection.LibraryConfig{lib("python", "2.7.3", "")}, "init_container (auto)"},
		{"falls back with an old python minor tag", []libraryinjection.LibraryConfig{lib("python", "v2.10", "")}, "init_container (auto)"},
		{"falls back with the python v1 major tag", []libraryinjection.LibraryConfig{lib("python", "v1", "")}, "init_container (auto)"},
		{"falls back with a java v0 version", []libraryinjection.LibraryConfig{lib("java", "v0.99.0", "")}, "init_container (auto)"},
		{"falls back with a js major line older than the first one with the layout", []libraryinjection.LibraryConfig{lib("js", "v2.0.0", "")}, "init_container (auto)"},
		{"falls back with the js v3 major tag", []libraryinjection.LibraryConfig{lib("js", "v3", "")}, "init_container (auto)"},
		{"falls back with an old js v4 version", []libraryinjection.LibraryConfig{lib("js", "v4.44.0", "")}, "init_container (auto)"},
		{"falls back with an old js v5 version", []libraryinjection.LibraryConfig{lib("js", "v5.20.0", "")}, "init_container (auto)"},
		{"falls back with a prerelease of the first java version with the layout", []libraryinjection.LibraryConfig{lib("java", "v1.38.0-rc1", "")}, "init_container (auto)"},
		{"falls back with a digest and an old canonical version", []libraryinjection.LibraryConfig{lib("java", "sha256:abc", "v1.37.1")}, "init_container (auto)"},
		{"falls back when one library is too old", []libraryinjection.LibraryConfig{lib("java", "v1", ""), lib("dotnet", "v2.56.0", "")}, "init_container (auto)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := libraryinjection.NewAutoProvider(libraryinjection.LibraryInjectionConfig{
				Injector:          injectorConfig(),
				Libraries:         tt.libraries,
				CSIAutoRegistries: defaultCSIAutoRegistries,
				CSIDriverWatcher:  fakeCSIDriverWatcher{registered: true, apmEnabled: true},
				KubeServerVersion: csiKubeVersion,
			})
			require.Equal(t, tt.wantMode, provider.GetName())
		})
	}
}

func TestAutoProvider_Reason(t *testing.T) {
	csiConfig := func() libraryinjection.LibraryInjectionConfig {
		return libraryinjection.LibraryInjectionConfig{
			Injector:          injectorConfig(),
			CSIAutoRegistries: defaultCSIAutoRegistries,
			CSIDriverWatcher:  fakeCSIDriverWatcher{registered: true, apmEnabled: true},
			KubeServerVersion: csiKubeVersion,
		}
	}
	tests := []struct {
		name       string
		update     func(*libraryinjection.LibraryInjectionConfig)
		wantReason string
	}{
		{"csi", func(*libraryinjection.LibraryInjectionConfig) {}, "all CSI requirements are met"},
		{"no csi driver", func(cfg *libraryinjection.LibraryInjectionConfig) { cfg.CSIDriverWatcher = nil }, "the CSI driver is not installed or APM is not enabled"},
		{"openshift", func(cfg *libraryinjection.LibraryInjectionConfig) { cfg.IsOpenShift = true }, "the cluster runs OpenShift"},
		{"unknown kubernetes version", func(cfg *libraryinjection.LibraryInjectionConfig) { cfg.KubeServerVersion = nil }, "the Kubernetes version is unknown"},
		{"unparsable kubernetes version", func(cfg *libraryinjection.LibraryInjectionConfig) { cfg.KubeServerVersion = &version.Info{} }, "the Kubernetes version is unknown"},
		{"old kubernetes version", func(cfg *libraryinjection.LibraryInjectionConfig) {
			cfg.KubeServerVersion = &version.Info{GitVersion: "v1.19.16"}
		}, "Kubernetes v1.19.16 is older than v1.20"},
		{"unsupported registry", func(cfg *libraryinjection.LibraryInjectionConfig) {
			cfg.Injector.Package.Registry = "registry.example.com/datadog"
		}, "registry registry.example.com/datadog is not a Datadog public registry"},
		{"old library version", func(cfg *libraryinjection.LibraryInjectionConfig) {
			cfg.Libraries = []libraryinjection.LibraryConfig{{
				Language: "python",
				Package:  libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/dd-lib-python-init:v2.10.4", ""),
			}}
		}, "python library version v2.10.4 predates CSI support"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := csiConfig()
			tt.update(&cfg)
			require.Equal(t, tt.wantReason, libraryinjection.NewAutoProvider(cfg).Reason())
		})
	}
}

func TestAutoProvider_KubernetesVersion(t *testing.T) {
	tests := []struct {
		name          string
		serverVersion *version.Info
		wantMode      string
	}{
		{"uses csi on 1.20", &version.Info{GitVersion: "v1.20.0"}, "csi (auto)"},
		{"uses csi on a 1.20 GKE version", &version.Info{GitVersion: "v1.20.0-gke.1"}, "csi (auto)"},
		{"falls back on 1.19", &version.Info{GitVersion: "v1.19.16"}, "init_container (auto)"},
		{"falls back on a 1.19 EKS version", &version.Info{GitVersion: "v1.19.16-eks-abc"}, "init_container (auto)"},
		{"falls back when the version is unknown", nil, "init_container (auto)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := libraryinjection.NewAutoProvider(libraryinjection.LibraryInjectionConfig{
				Injector:          injectorConfig(),
				CSIAutoRegistries: defaultCSIAutoRegistries,
				CSIDriverWatcher:  fakeCSIDriverWatcher{registered: true, apmEnabled: true},
				KubeServerVersion: tt.serverVersion,
			})
			require.Equal(t, tt.wantMode, provider.GetName())
		})
	}
}
