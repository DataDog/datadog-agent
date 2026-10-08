// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_apps

import (
	"encoding/json"
	"errors"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func (suite *AppsTestSuite) TestScaleStatefulSetHorizontally() {
	testNamespace := "test-namespace"
	testStatefulSetName := "test-stateful-set"

	testCases := []struct {
		name             string
		inputs           map[string]any
		expectedError    error
		expectedReplicas float64
	}{
		{
			name: "successful scale up",
			inputs: map[string]any{
				"namespace": testNamespace,
				"name":      testStatefulSetName,
				"replicas":  5,
			},
			expectedReplicas: 5,
		},
		{
			name: "scale to zero",
			inputs: map[string]any{
				"namespace": testNamespace,
				"name":      testStatefulSetName,
				"replicas":  0,
			},
			expectedReplicas: 0,
		},
		{
			name: "stateful set not found",
			inputs: map[string]any{
				"namespace": testNamespace,
				"name":      "nonexistent-stateful-set",
				"replicas":  3,
			},
			expectedError: errors.New("not found"),
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			clientSet := fake.NewSimpleClientset(createTestStatefulSet(testStatefulSetName, testNamespace))
			handler := &ScaleStatefulSetHorizontallyHandler{c: clientSet}

			outputs, err := handler.Run(testContext, newTestTask(tc.inputs), newTestCredentials())

			if tc.expectedError != nil {
				suite.Error(err, "expected error for test case: %s", tc.name)
				suite.Nil(outputs, "outputs should be nil on error for test case: %s", tc.name)
				suite.Contains(err.Error(), tc.expectedError.Error(), "error message should contain expected error for test case: %s", tc.name)
				return
			}

			suite.NoError(err, "unexpected error for test case: %s", tc.name)
			suite.NotNil(outputs, "outputs should not be nil for test case: %s", tc.name)

			actions := clientSet.Actions()
			suite.Len(actions, 1, "should only have a Patch operation for test case: %s", tc.name)

			patchAction := actions[0].(ktesting.PatchAction)
			suite.Equal("statefulsets", patchAction.GetResource().Resource)
			suite.Equal(testNamespace, patchAction.GetNamespace())
			suite.Equal(testStatefulSetName, patchAction.GetName())
			suite.Equal(types.MergePatchType, patchAction.GetPatchType())

			var patch map[string]interface{}
			suite.NoError(json.Unmarshal(patchAction.GetPatch(), &patch))
			suite.Equal(tc.expectedReplicas, patch["spec"].(map[string]interface{})["replicas"])
		})
	}
}

func createTestStatefulSet(name, namespace string) *appsv1.StatefulSet {
	replicas := int32(1)
	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			Labels:          map[string]string{"app": "test"},
			ResourceVersion: "1",
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: name,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "app-container",
							Image: "nginx:latest",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
						{
							Name:  "sidecar-container",
							Image: "alpine:latest",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU: resource.MustParse("50m"),
								},
							},
						},
					},
				},
			},
		},
		Status: appsv1.StatefulSetStatus{
			Replicas:      replicas,
			ReadyReplicas: replicas,
		},
	}
}
