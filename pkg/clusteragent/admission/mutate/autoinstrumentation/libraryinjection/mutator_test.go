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
