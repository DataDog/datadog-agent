// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/ssi/testutils"
)

func TestPodValidator(t *testing.T) {
	tests := map[string]struct {
		in      *corev1.Pod
		require func(t *testing.T, v *testutils.PodValidator)
	}{
		"ensure annotations match expected": {
			in: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"foo": "bar",
					},
				},
			},
			require: func(t *testing.T, v *testutils.PodValidator) {
				v.RequireAnnotations(t, map[string]string{
					"foo": "bar",
				})
			},
		},
		"ensure volume names match expected": {
			in: &corev1.Pod{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "foo",
						},
						{
							Name: "bar",
						},
					},
				},
			},
			require: func(t *testing.T, v *testutils.PodValidator) {
				v.RequireVolumeNames(t, []string{"foo", "bar"})
			},
		},
		"ensure init container versions are parsed from tags and digests": {
			in: &corev1.Pod{
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						{Name: "datadog-init-apm-inject", Image: "registry.datadoghq.com/apm-inject:0.54.0"},
						{Name: "datadog-lib-java-init", Image: "gcr.io/datadoghq/dd-lib-java-init:v1"},
						{Name: "datadog-lib-python-init", Image: "registry.local:5000/dd-lib-python-init@sha256:abc"},
					},
				},
			},
			require: func(t *testing.T, v *testutils.PodValidator) {
				v.RequireInjectorVersion(t, "0.54.0")
				v.RequireLibraryVersions(t, map[string]string{"java": "v1", "python": "sha256:abc"})
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			v := testutils.NewPodValidator(test.in, testutils.InjectionModeAuto)
			test.require(t, v)
		})
	}
}
