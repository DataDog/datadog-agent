// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/ssi/testutils"
)

func TestContainerValidator(t *testing.T) {
	tests := map[string]struct {
		in      *corev1.Container
		require func(t *testing.T, v *testutils.ContainerValidator)
	}{
		"ensure env matches expected": {
			in: &corev1.Container{
				Env: []corev1.EnvVar{
					{
						Name:  "foo",
						Value: "bar",
					},
				},
			},
			require: func(t *testing.T, v *testutils.ContainerValidator) {
				v.RequireEnvs(t, map[string]string{
					"foo": "bar",
				})
				v.RequireMissingEnvs(t, []string{"zed"})
			},
		},
		"ensure command matches expected": {
			in: &corev1.Container{
				Command: []string{"foo", "bar"},
				Args:    []string{"zed", "idk"},
			},
			require: func(t *testing.T, v *testutils.ContainerValidator) {
				v.RequireCommand(t, "foo bar zed idk")
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			v := testutils.NewContainerValidator(test.in, nil)
			test.require(t, v)
		})
	}
}
