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
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	wmutil "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const (
	datadogCSIDriverName = "k8s.csi.datadoghq.com"
	// csiAPMEnabledAnnotation mirrors the const of the same name in auto.go.
	// We intentionally hardcode the value here rather than exporting it so the
	// tests catch any accidental rename of the production string.
	csiAPMEnabledAnnotation = "csi.datadoghq.com/apm-enabled"
)

// newDatadogCSIDriverEntity builds a workloadmeta KubernetesMetadata entity
// matching what the kubeapiserver collector would produce for the Datadog
// CSIDriver object.
func newDatadogCSIDriverEntity(annotations map[string]string) *workloadmeta.KubernetesMetadata {
	id := wmutil.GenerateKubeMetadataEntityID("storage.k8s.io", "csidrivers", "", datadogCSIDriverName)
	return &workloadmeta.KubernetesMetadata{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesMetadata,
			ID:   string(id),
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:        datadogCSIDriverName,
			Annotations: annotations,
		},
	}
}

// newMockWorkloadmeta returns a workloadmeta mock store usable in tests.
func newMockWorkloadmeta(t *testing.T) workloadmetamock.Mock {
	t.Helper()
	return fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() config.Component { return config.NewMock(t) }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
}

func TestAutoProvider_PicksCSIWhenDatadogCSIDriverRegistered(t *testing.T) {
	wmeta := newMockWorkloadmeta(t)
	wmeta.Set(newDatadogCSIDriverEntity(map[string]string{csiAPMEnabledAnnotation: "true"}))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "my-app:latest"}},
		},
	}

	provider := libraryinjection.NewAutoProvider(libraryinjection.LibraryInjectionConfig{
		Wmeta:                   wmeta,
		CSIAutoDetectionEnabled: true,
	})

	result := provider.InjectInjector(pod, libraryinjection.InjectorConfig{
		Package: libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/apm-inject:0.52.0", "0.52.0"),
	})
	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	// CSIProvider injects the InstrumentationVolume as a CSI volume; the
	// init-container provider would have used an EmptyDir instead. Use that
	// difference as the discriminating signal between the two strategies.
	r := require.New(t)
	r.NotEmpty(pod.Spec.Volumes, "expected at least one volume to be added")
	var instrumentationVol *corev1.Volume
	for i := range pod.Spec.Volumes {
		if pod.Spec.Volumes[i].Name == libraryinjection.InstrumentationVolumeName {
			instrumentationVol = &pod.Spec.Volumes[i]
			break
		}
	}
	r.NotNil(instrumentationVol, "instrumentation volume should have been added")
	r.NotNil(instrumentationVol.CSI, "instrumentation volume should be a CSI volume")
	r.Equal(datadogCSIDriverName, instrumentationVol.CSI.Driver)
}

func TestAutoProvider_FallsBackToInitContainerWhenCSIDriverMissing(t *testing.T) {
	wmeta := newMockWorkloadmeta(t)
	// no CSI driver registered

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "my-app:latest"}},
		},
	}

	provider := libraryinjection.NewAutoProvider(libraryinjection.LibraryInjectionConfig{
		Wmeta:                   wmeta,
		CSIAutoDetectionEnabled: true,
	})

	result := provider.InjectInjector(pod, libraryinjection.InjectorConfig{
		Package: libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/apm-inject:0.52.0", "0.52.0"),
	})
	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	// The init-container provider creates an EmptyDir volume for the
	// instrumentation directory.
	for _, v := range pod.Spec.Volumes {
		if v.Name == libraryinjection.InstrumentationVolumeName {
			assert.Nil(t, v.CSI, "instrumentation volume should not be a CSI volume when driver is missing")
			assert.NotNil(t, v.EmptyDir, "instrumentation volume should be an EmptyDir when CSI driver is missing")
			return
		}
	}
	t.Fatalf("instrumentation volume %q not found", libraryinjection.InstrumentationVolumeName)
}

func TestAutoProvider_FallsBackToInitContainerWhenWmetaIsNil(t *testing.T) {
	// Defensive case: AutoProvider must never panic when wmeta is unset, even
	// with CSI auto-detection turned on.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "my-app:latest"}},
		},
	}

	provider := libraryinjection.NewAutoProvider(libraryinjection.LibraryInjectionConfig{
		CSIAutoDetectionEnabled: true,
	})

	result := provider.InjectInjector(pod, libraryinjection.InjectorConfig{
		Package: libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/apm-inject:0.52.0", "0.52.0"),
	})
	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	for _, v := range pod.Spec.Volumes {
		if v.Name == libraryinjection.InstrumentationVolumeName {
			assert.Nil(t, v.CSI, "with nil wmeta, AutoProvider must not produce CSI volumes")
			assert.NotNil(t, v.EmptyDir, "with nil wmeta, AutoProvider must fall back to an EmptyDir volume")
			return
		}
	}
	t.Fatalf("instrumentation volume %q not found", libraryinjection.InstrumentationVolumeName)
}

func TestAutoProvider_FallsBackToInitContainerWhenAPMAnnotationMissing(t *testing.T) {
	// The Datadog CSI driver may be installed for purposes other than APM
	// auto-instrumentation. We only switch to the CSI provider when the driver
	// explicitly opts in via the apm-enabled annotation.
	wmeta := newMockWorkloadmeta(t)
	// No apm-enabled annotation.
	wmeta.Set(newDatadogCSIDriverEntity(nil))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "my-app:latest"}},
		},
	}

	provider := libraryinjection.NewAutoProvider(libraryinjection.LibraryInjectionConfig{
		Wmeta:                   wmeta,
		CSIAutoDetectionEnabled: true,
	})

	result := provider.InjectInjector(pod, libraryinjection.InjectorConfig{
		Package: libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/apm-inject:0.52.0", "0.52.0"),
	})
	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	for _, v := range pod.Spec.Volumes {
		if v.Name == libraryinjection.InstrumentationVolumeName {
			assert.Nil(t, v.CSI, "without apm-enabled annotation, instrumentation volume must not be a CSI volume")
			assert.NotNil(t, v.EmptyDir, "without apm-enabled annotation, instrumentation volume must be an EmptyDir")
			return
		}
	}
	t.Fatalf("instrumentation volume %q not found", libraryinjection.InstrumentationVolumeName)
}

func TestAutoProvider_FallsBackToInitContainerWhenAPMAnnotationNotTrue(t *testing.T) {
	// The annotation is present but explicitly disables APM (any value other
	// than "true" must be treated as opt-out).
	wmeta := newMockWorkloadmeta(t)
	wmeta.Set(newDatadogCSIDriverEntity(map[string]string{csiAPMEnabledAnnotation: "false"}))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "my-app:latest"}},
		},
	}

	provider := libraryinjection.NewAutoProvider(libraryinjection.LibraryInjectionConfig{
		Wmeta:                   wmeta,
		CSIAutoDetectionEnabled: true,
	})

	result := provider.InjectInjector(pod, libraryinjection.InjectorConfig{
		Package: libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/apm-inject:0.52.0", "0.52.0"),
	})
	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	for _, v := range pod.Spec.Volumes {
		if v.Name == libraryinjection.InstrumentationVolumeName {
			assert.Nil(t, v.CSI, "with apm-enabled=false, instrumentation volume must not be a CSI volume")
			assert.NotNil(t, v.EmptyDir, "with apm-enabled=false, instrumentation volume must be an EmptyDir")
			return
		}
	}
	t.Fatalf("instrumentation volume %q not found", libraryinjection.InstrumentationVolumeName)
}

func TestAutoProvider_FallsBackToInitContainerWhenFlagDisabled(t *testing.T) {
	// Even with the Datadog CSI driver registered AND APM-enabled on it, the
	// auto provider must stay on the init-container path while the temporary
	// feature flag is off.
	wmeta := newMockWorkloadmeta(t)
	wmeta.Set(newDatadogCSIDriverEntity(map[string]string{csiAPMEnabledAnnotation: "true"}))

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "test-pod", Namespace: "default"},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app", Image: "my-app:latest"}},
		},
	}

	provider := libraryinjection.NewAutoProvider(libraryinjection.LibraryInjectionConfig{
		Wmeta:                   wmeta,
		CSIAutoDetectionEnabled: false,
	})

	result := provider.InjectInjector(pod, libraryinjection.InjectorConfig{
		Package: libraryinjection.NewLibraryImageFromFullRef("gcr.io/datadoghq/apm-inject:0.52.0", "0.52.0"),
	})
	assert.Equal(t, libraryinjection.MutationStatusInjected, result.Status)

	for _, v := range pod.Spec.Volumes {
		if v.Name == libraryinjection.InstrumentationVolumeName {
			assert.Nil(t, v.CSI, "flag is off: instrumentation volume must not be a CSI volume")
			assert.NotNil(t, v.EmptyDir, "flag is off: instrumentation volume must be an EmptyDir")
			return
		}
	}
	t.Fatalf("instrumentation volume %q not found", libraryinjection.InstrumentationVolumeName)
}
