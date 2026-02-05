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
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			v := testutils.NewPodValidator(test.in, testutils.InjectionModeAuto)
			test.require(t, v)
		})
	}
}
