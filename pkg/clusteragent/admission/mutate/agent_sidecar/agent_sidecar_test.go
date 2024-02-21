// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

import (
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"testing"
)

func TestInjectAgentSidecar(t *testing.T) {
	mockConfig := config.Mock(t)

	tests := []struct {
		Name                      string
		Pod                       *corev1.Pod
		provider                  string
		ExpectError               bool
		ExpectedPodAfterInjection *corev1.Pod
	}{
		{
			Name:                      "should return error for nil pod",
			Pod:                       nil,
			provider:                  "",
			ExpectError:               true,
			ExpectedPodAfterInjection: nil,
		},
		{
			Name: "should inject sidecar if no sidecar present, no provider set",
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-name",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "container-name"},
					},
				},
			},
			provider:    "",
			ExpectError: false,
			ExpectedPodAfterInjection: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-name",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "container-name"},
						*getDefaultSidecarTemplate(),
					},
				},
			},
		},
		{
			Name: "should skip injecting sidecar when sidecar already exists",
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-name",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "container-name"},
						*getDefaultSidecarTemplate(),
					},
				},
			},
			provider:    "",
			ExpectError: false,
			ExpectedPodAfterInjection: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-name",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "container-name"},
						*getDefaultSidecarTemplate(),
					},
				},
			},
		},
		{
			Name: "should inject sidecar if no sidecar present, with supported provider",
			Pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-name",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "container-name"},
					},
				},
			},
			provider:    "fargate",
			ExpectError: false,
			ExpectedPodAfterInjection: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name: "pod-name",
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "container-name"},
						*sidecarWithEnvOverrides(corev1.EnvVar{
							Name:  "DD_EKS_FARGATE",
							Value: "true",
						}),
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(tt *testing.T) {
			mockConfig.SetWithoutSource("admission_controller.agent_sidecar.provider", test.provider)

			err := injectAgentSidecar(test.Pod, "", nil)

			if test.ExpectError {
				assert.Error(tt, err, "expected non-nil error to be returned")
			} else {
				assert.NoError(tt, err, "expected returned error to be nil")
			}

			if test.ExpectedPodAfterInjection == nil {
				assert.Nil(tt, test.Pod)
			} else {
				assert.NotNil(tt, test.Pod)
				assert.Truef(
					tt,
					reflect.DeepEqual(*test.ExpectedPodAfterInjection, *test.Pod),
					"expected %v, found %v",
					*test.ExpectedPodAfterInjection,
					*test.Pod,
				)
			}

		})
	}

}

func sidecarWithEnvOverrides(extraEnv ...corev1.EnvVar) *corev1.Container {
	sidecar := getDefaultSidecarTemplate()
	sidecar.Env = append(sidecar.Env, extraEnv...)
	return sidecar
}
