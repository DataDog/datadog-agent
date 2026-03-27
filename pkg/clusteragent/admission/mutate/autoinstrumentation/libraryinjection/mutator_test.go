// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package libraryinjection_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/version"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/libraryinjection"
)

func TestInjectAPMLibraries_RegistryAllowList(t *testing.T) {
	tests := []struct {
		name              string
		registryAllowList []string
		injectorRegistry  string
		expectInjected    bool
		expectErrorAnnot  bool
	}{
		{
			name:              "empty allow list permits any registry",
			registryAllowList: []string{},
			injectorRegistry:  "registry.datadoghq.com",
			expectInjected:    true,
			expectErrorAnnot:  false,
		},
		{
			name:              "registry in allow list permits injection",
			registryAllowList: []string{"gcr.io/datadoghq", "public.ecr.aws/datadog"},
			injectorRegistry:  "gcr.io/datadoghq",
			expectInjected:    true,
			expectErrorAnnot:  false,
		},
		{
			name:              "registry not in allow list blocks injection",
			registryAllowList: []string{"fake.registry.invalid"},
			injectorRegistry:  "registry.datadoghq.com",
			expectInjected:    false,
			expectErrorAnnot:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pod",
					Namespace: "default",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app"}},
				},
			}

			err := libraryinjection.InjectAPMLibraries(pod, libraryinjection.LibraryInjectionConfig{
				InjectionMode:     "auto",
				KubeServerVersion: &version.Info{GitVersion: "v1.30.9"},
				RegistryAllowList: tt.registryAllowList,
				Injector: libraryinjection.InjectorConfig{
					Package: libraryinjection.NewLibraryImageFromFullRef(tt.injectorRegistry+"/apm-inject:0.52.0", "0.52.0"),
				},
			})

			require.NoError(t, err)

			_, hasErrorAnnot := annotation.Get(pod, annotation.InjectionError)
			require.Equal(t, tt.expectErrorAnnot, hasErrorAnnot)

			if tt.expectErrorAnnot {
				val, _ := annotation.Get(pod, annotation.InjectionError)
				require.Contains(t, val, "not in the allow list")
			}

			// Verify injection state: check whether LD_PRELOAD was set on the container.
			injected := false
			for _, env := range pod.Spec.Containers[0].Env {
				if env.Name == "LD_PRELOAD" {
					injected = true
					break
				}
			}
			require.Equal(t, tt.expectInjected, injected)
		})
	}
}

func TestInjectAPMLibraries_RegistryAllowListBlocksLibraryRegistry(t *testing.T) {
	// A library specified via custom-image annotation may come from a different registry
	// than the injector. The allow list must also cover library registries.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "app"}},
		},
	}

	err := libraryinjection.InjectAPMLibraries(pod, libraryinjection.LibraryInjectionConfig{
		InjectionMode:     "auto",
		KubeServerVersion: &version.Info{GitVersion: "v1.30.9"},
		RegistryAllowList: []string{"registry.datadoghq.com"},
		Injector: libraryinjection.InjectorConfig{
			Package: libraryinjection.NewLibraryImageFromFullRef("registry.datadoghq.com/apm-inject:0.52.0", "0.52.0"),
		},
		Libraries: []libraryinjection.LibraryConfig{
			{
				Language: "python",
				Package:  libraryinjection.NewLibraryImageFromFullRef("evil.registry.invalid/dd-lib-python-init:v3.18.1", "v3.18.1"),
			},
		},
	})
	require.NoError(t, err)

	val, ok := annotation.Get(pod, annotation.InjectionError)
	require.True(t, ok, "expected injection-error annotation to be set")
	require.Contains(t, val, "not in the allow list")

	// Injection should be blocked — LD_PRELOAD should not be set.
	for _, env := range pod.Spec.Containers[0].Env {
		require.NotEqual(t, "LD_PRELOAD", env.Name, "expected LD_PRELOAD to not be set")
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
