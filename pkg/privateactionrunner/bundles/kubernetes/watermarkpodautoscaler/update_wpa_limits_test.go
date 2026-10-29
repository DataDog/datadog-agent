// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_kubernetes_watermarkpodautoscaler

import (
	"encoding/json"
	"errors"

	"github.com/DataDog/watermarkpodautoscaler/apis/datadoghq/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

func (suite *WatermarkPodAutoscalerTestSuite) TestUpdateWatermarkAutoscalerLimits() {
	const (
		namespace = "production"
		wpaName   = "checkout"
	)

	tests := []struct {
		name            string
		inputs          map[string]any
		wpaExists       bool
		expectedError   string
		expectedMinimum int32
		expectedMaximum int32
		expectedPatch   map[string]int32
	}{
		{
			name:            "updates both bounds",
			inputs:          map[string]any{"minReplicas": 4, "maxReplicas": 12},
			wpaExists:       true,
			expectedMinimum: 4,
			expectedMaximum: 12,
			expectedPatch:   map[string]int32{"minReplicas": 4, "maxReplicas": 12},
		},
		{
			name:            "updates only minimum",
			inputs:          map[string]any{"minReplicas": 4},
			wpaExists:       true,
			expectedMinimum: 4,
			expectedMaximum: 10,
			expectedPatch:   map[string]int32{"minReplicas": 4},
		},
		{
			name:            "updates only maximum",
			inputs:          map[string]any{"maxReplicas": 20},
			wpaExists:       true,
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
			// A WatermarkPodAutoscaler cannot scale to zero: its minReplicas
			// carries kubebuilder:validation:Minimum=1, unlike a HorizontalPodAutoscaler.
			name:          "rejects zero minimum",
			inputs:        map[string]any{"minReplicas": 0},
			expectedError: "minReplicas must be greater than or equal to 1",
		},
		{
			name:          "rejects negative minimum",
			inputs:        map[string]any{"minReplicas": -1},
			expectedError: "minReplicas must be greater than or equal to 1",
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
			// Both bounds are supplied so the handler patches without reading
			// the live object first, which is what surfaces the patch error.
			name:          "returns Kubernetes error",
			inputs:        map[string]any{"minReplicas": 1, "maxReplicas": 10},
			expectedError: "not found",
			expectedPatch: map[string]int32{"minReplicas": 1, "maxReplicas": 10},
		},
		{
			// The CRD has no cross-field validation, so a single-bound patch must
			// be checked against the live object: persisting maxReplicas <
			// minReplicas makes the WPA controller stop autoscaling the target.
			name:          "rejects minimum above the existing maximum",
			inputs:        map[string]any{"minReplicas": 50},
			wpaExists:     true,
			expectedError: "minReplicas must be less than or equal to maxReplicas",
		},
		{
			name:          "rejects maximum below the existing minimum",
			inputs:        map[string]any{"maxReplicas": 1},
			wpaExists:     true,
			expectedError: "minReplicas must be less than or equal to maxReplicas",
		},
		{
			name:          "reports a read failure when the autoscaler is absent",
			inputs:        map[string]any{"maxReplicas": 20},
			expectedError: "not found",
		},
	}

	for _, test := range tests {
		suite.Run(test.name, func() {
			scheme := runtime.NewScheme()
			suite.Require().NoError(v1alpha1.AddToScheme(scheme))

			objects := []runtime.Object{}
			if test.wpaExists {
				minimumReplicas := int32(2)
				objects = append(objects, &v1alpha1.WatermarkPodAutoscaler{
					TypeMeta: metav1.TypeMeta{
						APIVersion: v1alpha1.GroupVersion.String(),
						Kind:       "WatermarkPodAutoscaler",
					},
					ObjectMeta: metav1.ObjectMeta{Name: wpaName, Namespace: namespace},
					Spec: v1alpha1.WatermarkPodAutoscalerSpec{
						MinReplicas: &minimumReplicas,
						MaxReplicas: 10,
					},
				})
			}

			client := dynamicfake.NewSimpleDynamicClient(scheme, objects...)
			handler := &UpdateWatermarkAutoscalerLimitsHandler{client: client}
			inputs := map[string]any{"namespace": namespace, "name": wpaName}
			for key, value := range test.inputs {
				inputs[key] = value
			}

			outputs, err := handler.Run(testContext, newTestTask(inputs), newTestCredentials())

			if test.expectedError != "" {
				suite.Require().ErrorContains(err, test.expectedError)
				suite.Nil(outputs)
			} else {
				suite.Require().NoError(err)
				output, ok := outputs.(*UpdateWatermarkAutoscalerLimitsOutputs)
				suite.Require().True(ok)
				suite.Require().NotNil(output.WatermarkPodAutoscaler.Spec.MinReplicas)
				suite.Equal(test.expectedMinimum, *output.WatermarkPodAutoscaler.Spec.MinReplicas)
				suite.Equal(test.expectedMaximum, output.WatermarkPodAutoscaler.Spec.MaxReplicas)
			}

			var patchActions []ktesting.PatchAction
			for _, action := range client.Actions() {
				if patch, ok := action.(ktesting.PatchAction); ok && action.GetVerb() == "patch" {
					patchActions = append(patchActions, patch)
				}
			}

			if test.expectedPatch == nil {
				suite.Empty(patchActions, "expected no patch to be issued")
				return
			}

			suite.Require().Len(patchActions, 1)
			patchAction := patchActions[0]
			suite.Equal("watermarkpodautoscalers", patchAction.GetResource().Resource)
			suite.Equal("datadoghq.com", patchAction.GetResource().Group)
			suite.Equal(namespace, patchAction.GetNamespace())
			suite.Equal(wpaName, patchAction.GetName())
			suite.Equal(types.MergePatchType, patchAction.GetPatchType())

			var patch struct {
				Spec map[string]int32 `json:"spec"`
			}
			suite.Require().NoError(json.Unmarshal(patchAction.GetPatch(), &patch))
			suite.Equal(test.expectedPatch, patch.Spec)
		})
	}
}

// TestUpdateWatermarkAutoscalerLimitsConcurrency covers the optimistic-concurrency
// precondition. Without it the bound read by validateAgainstExisting can change
// before the patch lands: two concurrent single-bound calls on min=2/max=10,
// one setting minReplicas=8 and one setting maxReplicas=5, would each validate
// against a stale pair and together persist min=8/max=5. The CRD has no
// cross-field validation, so nothing rejects that and the controller stops
// autoscaling the target.
func (suite *WatermarkPodAutoscalerTestSuite) TestUpdateWatermarkAutoscalerLimitsConcurrency() {
	const (
		namespace       = "production"
		wpaName         = "checkout"
		resourceVersion = "4242"
	)

	newClient := func() *dynamicfake.FakeDynamicClient {
		scheme := runtime.NewScheme()
		suite.Require().NoError(v1alpha1.AddToScheme(scheme))
		minimumReplicas := int32(2)
		return dynamicfake.NewSimpleDynamicClient(scheme, &v1alpha1.WatermarkPodAutoscaler{
			TypeMeta: metav1.TypeMeta{
				APIVersion: v1alpha1.GroupVersion.String(),
				Kind:       "WatermarkPodAutoscaler",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:            wpaName,
				Namespace:       namespace,
				ResourceVersion: resourceVersion,
			},
			Spec: v1alpha1.WatermarkPodAutoscalerSpec{
				MinReplicas: &minimumReplicas,
				MaxReplicas: 10,
			},
		})
	}

	patchBody := func(client *dynamicfake.FakeDynamicClient) map[string]interface{} {
		for _, action := range client.Actions() {
			if patch, ok := action.(ktesting.PatchAction); ok && action.GetVerb() == "patch" {
				var body map[string]interface{}
				suite.Require().NoError(json.Unmarshal(patch.GetPatch(), &body))
				return body
			}
		}
		suite.Require().Fail("no patch was issued")
		return nil
	}

	runWith := func(client *dynamicfake.FakeDynamicClient, extra map[string]any) (interface{}, error) {
		handler := &UpdateWatermarkAutoscalerLimitsHandler{client: client}
		inputs := map[string]any{"namespace": namespace, "name": wpaName}
		for key, value := range extra {
			inputs[key] = value
		}
		return handler.Run(testContext, newTestTask(inputs), newTestCredentials())
	}

	suite.Run("a single-bound patch carries the resourceVersion it read", func() {
		client := newClient()
		_, err := runWith(client, map[string]any{"minReplicas": 4})
		suite.Require().NoError(err)

		metadata, ok := patchBody(client)["metadata"].(map[string]interface{})
		suite.Require().True(ok, "expected a metadata precondition in the patch")
		suite.Equal(resourceVersion, metadata["resourceVersion"])
	})

	suite.Run("a both-bounds patch carries no precondition", func() {
		client := newClient()
		_, err := runWith(client, map[string]any{"minReplicas": 4, "maxReplicas": 12})
		suite.Require().NoError(err)

		// Both bounds are set in one merge patch, so the result is self-consistent
		// without a read and last-writer-wins still leaves a valid pair.
		body := patchBody(client)
		suite.NotContains(body, "metadata")
	})

	suite.Run("a conflict surfaces an actionable error", func() {
		client := newClient()
		client.PrependReactor("patch", "watermarkpodautoscalers", func(ktesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewConflict(
				v1alpha1.GroupVersion.WithResource("watermarkpodautoscalers").GroupResource(),
				wpaName,
				errors.New("the object has been modified"),
			)
		})

		outputs, err := runWith(client, map[string]any{"minReplicas": 4})
		suite.Require().Error(err)
		suite.Nil(outputs)
		suite.ErrorContains(err, "was modified while this action was running")
		suite.ErrorContains(err, "re-run the action")
		suite.True(apierrors.IsConflict(err), "the underlying conflict should stay unwrapped-to")
	})
}
