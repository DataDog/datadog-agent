// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_apps

import (
	"errors"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func (suite *AppsTestSuite) TestScaleStatefulSetVertically() {
	testNamespace := "test-namespace"
	testStatefulSetName := "test-stateful-set"

	statefulSet := createTestStatefulSet(testStatefulSetName, testNamespace)
	clientSet := fake.NewSimpleClientset(statefulSet)

	handler := &ScaleStatefulSetVerticallyHandler{c: clientSet}

	testCases := []struct {
		name            string
		inputs          map[string]any
		expectedError   error
		expectedUpdates []map[string]interface{}
	}{
		{
			name: "successful resource scaling",
			inputs: map[string]any{
				"namespace": testNamespace,
				"name":      testStatefulSetName,
				"containerUpdates": []map[string]any{
					{
						"containerName": "app-container",
						"requests": map[string]any{
							"cpu":    "200m",
							"memory": "256Mi",
						},
						"limits": map[string]any{
							"cpu":    "500m",
							"memory": "512Mi",
						},
					},
				},
			},
			expectedUpdates: []map[string]interface{}{
				{
					"containerName": "app-container",
					"resources": map[string]interface{}{
						"requests": map[string]interface{}{
							"cpu":    "200m",
							"memory": "256Mi",
						},
						"limits": map[string]interface{}{
							"cpu":    "500m",
							"memory": "512Mi",
						},
					},
				},
			},
		},
		{
			name: "scale multiple containers",
			inputs: map[string]any{
				"namespace": testNamespace,
				"name":      testStatefulSetName,
				"containerUpdates": []map[string]any{
					{
						"containerName": "app-container",
						"requests": map[string]any{
							"cpu": "100m",
						},
					},
					{
						"containerName": "sidecar-container",
						"limits": map[string]any{
							"memory": "128Mi",
						},
					},
				},
			},
			expectedUpdates: []map[string]interface{}{
				{
					"containerName": "app-container",
					"resources": map[string]interface{}{
						"requests": map[string]interface{}{
							"cpu": "100m",
						},
					},
				},
				{
					"containerName": "sidecar-container",
					"resources": map[string]interface{}{
						"limits": map[string]interface{}{
							"memory": "128Mi",
						},
					},
				},
			},
		},
		{
			name: "empty containerUpdates",
			inputs: map[string]any{
				"namespace":        testNamespace,
				"name":             testStatefulSetName,
				"containerUpdates": []map[string]any{},
			},
			expectedError: errors.New("containerUpdates cannot be empty"),
		},
		{
			name: "empty container name",
			inputs: map[string]any{
				"namespace": testNamespace,
				"name":      testStatefulSetName,
				"containerUpdates": []map[string]any{
					{
						"containerName": "",
						"requests": map[string]any{
							"cpu": "100m",
						},
					},
				},
			},
			expectedError: errors.New("containerName cannot be empty"),
		},
		{
			name: "container not found",
			inputs: map[string]any{
				"namespace": testNamespace,
				"name":      testStatefulSetName,
				"containerUpdates": []map[string]any{
					{
						"containerName": "nonexistent-container",
						"requests": map[string]any{
							"cpu": "100m",
						},
					},
				},
			},
			expectedError: errors.New("container nonexistent-container not found in stateful set"),
		},
		{
			name: "invalid CPU request format",
			inputs: map[string]any{
				"namespace": testNamespace,
				"name":      testStatefulSetName,
				"containerUpdates": []map[string]any{
					{
						"containerName": "app-container",
						"requests": map[string]any{
							"cpu": "invalid-cpu",
						},
					},
				},
			},
			expectedError: errors.New("invalid CPU request format"),
		},
		{
			name: "invalid memory limit format",
			inputs: map[string]any{
				"namespace": testNamespace,
				"name":      testStatefulSetName,
				"containerUpdates": []map[string]any{
					{
						"containerName": "app-container",
						"limits": map[string]any{
							"memory": "invalid-memory",
						},
					},
				},
			},
			expectedError: errors.New("invalid memory limit format"),
		},
		{
			name: "stateful set not found",
			inputs: map[string]any{
				"namespace": testNamespace,
				"name":      "nonexistent-stateful-set",
				"containerUpdates": []map[string]any{
					{
						"containerName": "app-container",
						"requests": map[string]any{
							"cpu": "100m",
						},
					},
				},
			},
			expectedError: errors.New("failed to get stateful set"),
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			// Reset client actions for each test case
			clientSet.ClearActions()

			outputs, err := handler.Run(testContext, newTestTask(tc.inputs), newTestCredentials())

			if tc.expectedError != nil {
				suite.Error(err, "expected error for test case: %s", tc.name)
				suite.Nil(outputs, "outputs should be nil on error for test case: %s", tc.name)
				suite.Contains(err.Error(), tc.expectedError.Error(), "error message should contain expected error for test case: %s", tc.name)
			} else {
				suite.NoError(err, "unexpected error for test case: %s", tc.name)
				suite.NotNil(outputs, "outputs should not be nil for test case: %s", tc.name)

				// Verify the patch was applied correctly
				actions := clientSet.Actions()
				suite.Len(actions, 2, "should have Get + Patch operations for test case: %s", tc.name)

				patchAction := actions[1].(ktesting.PatchAction)
				suite.Equal("statefulsets", patchAction.GetResource().Resource, "patch resource should be statefulsets for test case: %s", tc.name)
				suite.Equal(testNamespace, patchAction.GetNamespace(), "patch namespace should match for test case: %s", tc.name)
				suite.Equal(testStatefulSetName, patchAction.GetName(), "patch name should match for test case: %s", tc.name)
				suite.Equal(types.StrategicMergePatchType, patchAction.GetPatchType(), "patch type should be strategic merge for test case: %s", tc.name)

				// Verify the patch content using JSON assertion
				suite.assertJSONPatch(patchAction.GetPatch(), tc.expectedUpdates)
			}
		})
	}
}
