// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_autoscaling

import (
	"encoding/json"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"
)

func (suite *AutoscalingTestSuite) TestUpdateAutoscalerLimits() {
	const (
		namespace = "production"
		hpaName   = "checkout"
	)

	tests := []struct {
		name            string
		inputs          map[string]any
		hpaExists       bool
		expectedError   string
		expectedMinimum int32
		expectedMaximum int32
		expectedPatch   map[string]int32
	}{
		{
			name:            "updates both bounds",
			inputs:          map[string]any{"minReplicas": 4, "maxReplicas": 12},
			hpaExists:       true,
			expectedMinimum: 4,
			expectedMaximum: 12,
			expectedPatch:   map[string]int32{"minReplicas": 4, "maxReplicas": 12},
		},
		{
			name:            "updates only minimum",
			inputs:          map[string]any{"minReplicas": 4},
			hpaExists:       true,
			expectedMinimum: 4,
			expectedMaximum: 10,
			expectedPatch:   map[string]int32{"minReplicas": 4},
		},
		{
			name:            "updates only maximum",
			inputs:          map[string]any{"maxReplicas": 20},
			hpaExists:       true,
			expectedMinimum: 2,
			expectedMaximum: 20,
			expectedPatch:   map[string]int32{"maxReplicas": 20},
		},
		{
			name:          "requires a bound",
			inputs:        map[string]any{},
			expectedError: "at least one of minReplicas or maxReplicas must be provided",
		},
		{
			name:          "rejects negative minimum",
			inputs:        map[string]any{"minReplicas": -1},
			expectedError: "minReplicas must be greater than or equal to 0",
		},
		{
			name:          "rejects non-positive maximum",
			inputs:        map[string]any{"maxReplicas": 0},
			expectedError: "maxReplicas must be greater than or equal to 1",
		},
		{
			name:          "rejects minimum greater than maximum",
			inputs:        map[string]any{"minReplicas": 5, "maxReplicas": 4},
			expectedError: "minReplicas must be less than or equal to maxReplicas",
		},
		{
			name:          "returns Kubernetes error",
			inputs:        map[string]any{"maxReplicas": 10},
			expectedError: "horizontalpodautoscalers.autoscaling \"checkout\" not found",
			expectedPatch: map[string]int32{"maxReplicas": 10},
		},
	}

	for _, test := range tests {
		suite.Run(test.name, func() {
			minimumReplicas := int32(2)
			client := fake.NewSimpleClientset()
			if test.hpaExists {
				client = fake.NewSimpleClientset(&autoscalingv2.HorizontalPodAutoscaler{
					ObjectMeta: metav1.ObjectMeta{Name: hpaName, Namespace: namespace},
					Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
						MinReplicas: &minimumReplicas,
						MaxReplicas: 10,
					},
				})
			}
			handler := &UpdateAutoscalerLimitsHandler{client: client}
			inputs := map[string]any{"namespace": namespace, "name": hpaName}
			for key, value := range test.inputs {
				inputs[key] = value
			}

			outputs, err := handler.Run(testContext, newTestTask(inputs), newTestCredentials())

			if test.expectedError != "" {
				suite.Require().EqualError(err, test.expectedError)
				suite.Nil(outputs)
			} else {
				suite.Require().NoError(err)
				output, ok := outputs.(*UpdateAutoscalerLimitsOutputs)
				suite.Require().True(ok)
				suite.Equal(test.expectedMinimum, *output.HorizontalPodAutoscaler.Spec.MinReplicas)
				suite.Equal(test.expectedMaximum, output.HorizontalPodAutoscaler.Spec.MaxReplicas)
			}

			if test.expectedPatch == nil {
				suite.Empty(client.Actions())
				return
			}

			actions := client.Actions()
			suite.Require().Len(actions, 1)
			patchAction, ok := actions[0].(ktesting.PatchAction)
			suite.Require().True(ok)
			suite.Equal("horizontalpodautoscalers", patchAction.GetResource().Resource)
			suite.Equal(namespace, patchAction.GetNamespace())
			suite.Equal(hpaName, patchAction.GetName())
			suite.Equal(types.MergePatchType, patchAction.GetPatchType())

			var patch struct {
				Spec map[string]int32 `json:"spec"`
			}
			suite.Require().NoError(json.Unmarshal(patchAction.GetPatch(), &patch))
			suite.Equal(test.expectedPatch, patch.Spec)
		})
	}
}
