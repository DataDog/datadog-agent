// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_apps

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func (suite *AppsTestSuite) TestScaleVerticallyContainerTypes() {
	testCases := []struct {
		name           string
		containerNames []string
		requestCPU     string
		limitMemory    string
		expectedError  string
	}{
		{name: "regular container", containerNames: []string{"app"}},
		{name: "init container", containerNames: []string{"setup"}},
		{name: "restartable init container", containerNames: []string{"init-sidecar"}},
		{name: "multiple init containers", containerNames: []string{"setup", "init-sidecar"}},
		{name: "mixed containers", containerNames: []string{"setup", "app", "init-sidecar"}},
		{
			name:           "unknown container after init container",
			containerNames: []string{"setup", "unknown"},
			expectedError:  "container unknown not found",
		},
		{
			name:           "empty container name",
			containerNames: []string{""},
			expectedError:  "containerName cannot be empty",
		},
		{
			name:           "invalid init container CPU request",
			containerNames: []string{"setup"},
			requestCPU:     "invalid-cpu",
			expectedError:  "invalid CPU request format",
		},
		{
			name:           "invalid init container memory limit",
			containerNames: []string{"setup"},
			limitMemory:    "invalid-memory",
			expectedError:  "invalid memory limit format",
		},
	}

	for _, kind := range []string{"deployment", "stateful set"} {
		suite.Run(kind, func() {
			for _, tc := range testCases {
				suite.Run(tc.name, func() {
					newContainer := func(name string) corev1.Container {
						return corev1.Container{
							Name:  name,
							Image: "test-image",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						}
					}
					restartPolicy := corev1.ContainerRestartPolicyAlways
					initSidecar := newContainer("init-sidecar")
					initSidecar.RestartPolicy = &restartPolicy
					podSpec := corev1.PodSpec{
						Containers:     []corev1.Container{newContainer("app"), newContainer("sidecar")},
						InitContainers: []corev1.Container{newContainer("setup"), initSidecar},
					}
					expectedPodSpec := podSpec.DeepCopy()
					updates := make([]map[string]any, 0, len(tc.containerNames))
					for _, name := range tc.containerNames {
						cpu, memory := "200m", "512Mi"
						if tc.requestCPU != "" {
							cpu = tc.requestCPU
						}
						if tc.limitMemory != "" {
							memory = tc.limitMemory
						}
						updates = append(updates, map[string]any{
							"containerName": name,
							"requests":      map[string]any{"cpu": cpu},
							"limits":        map[string]any{"memory": memory},
						})
						if tc.expectedError == "" {
							for _, containers := range [][]corev1.Container{expectedPodSpec.Containers, expectedPodSpec.InitContainers} {
								for i := range containers {
									if containers[i].Name == name {
										containers[i].Resources.Requests[corev1.ResourceCPU] = resource.MustParse(cpu)
										containers[i].Resources.Limits[corev1.ResourceMemory] = resource.MustParse(memory)
									}
								}
							}
						}
					}
					task := newTestTask(map[string]any{
						"namespace": "test-namespace", "name": "test-workload", "containerUpdates": updates,
					})
					metadata := metav1.ObjectMeta{Name: "test-workload", Namespace: "test-namespace"}
					template := corev1.PodTemplateSpec{Spec: podSpec}
					var client *fake.Clientset
					var output interface{}
					var err error
					if kind == "deployment" {
						client = fake.NewSimpleClientset(&appsv1.Deployment{
							ObjectMeta: metadata, Spec: appsv1.DeploymentSpec{Template: template},
						})
						output, err = (&ScaleDeploymentVerticallyHandler{c: client}).Run(testContext, task, newTestCredentials())
					} else {
						client = fake.NewSimpleClientset(&appsv1.StatefulSet{
							ObjectMeta: metadata, Spec: appsv1.StatefulSetSpec{Template: template},
						})
						output, err = (&ScaleStatefulSetVerticallyHandler{c: client}).Run(testContext, task, newTestCredentials())
					}
					if tc.expectedError != "" {
						suite.Require().ErrorContains(err, tc.expectedError)
						suite.Nil(output)
						suite.Require().Len(client.Actions(), 1, "invalid updates must not issue a patch")
						suite.Equal("get", client.Actions()[0].GetVerb())
						return
					}
					suite.Require().NoError(err)
					suite.Require().Len(client.Actions(), 2)
					patchAction := client.Actions()[1].(ktesting.PatchAction)
					suite.Equal(types.StrategicMergePatchType, patchAction.GetPatchType())
					var actualPodSpec corev1.PodSpec
					if kind == "deployment" {
						actualPodSpec = output.(*ScaleDeploymentVerticallyOutputs).Spec.Template.Spec
					} else {
						actualPodSpec = output.(*ScaleStatefulSetVerticallyOutputs).Spec.Template.Spec
					}
					// Check the merged result, including untouched resources, containers, and restart policy.
					suite.Equal(*expectedPodSpec, actualPodSpec)
				})
			}
		})
	}
}
