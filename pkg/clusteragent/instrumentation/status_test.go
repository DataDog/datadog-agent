// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package instrumentation

import (
	"testing"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestConditionFromHandlerStatus(t *testing.T) {
	tests := []struct {
		name       string
		status     HandlerStatus
		generation int64
		want       metav1.Condition
	}{
		{
			name: "true condition",
			status: HandlerStatus{
				Type:    "ChecksReady",
				Status:  metav1.ConditionTrue,
				Reason:  "Configured",
				Message: "all checks configured",
			},
			generation: 3,
			want: metav1.Condition{
				Type:               "ChecksReady",
				Status:             metav1.ConditionTrue,
				Reason:             "Configured",
				Message:            "all checks configured",
				ObservedGeneration: 3,
			},
		},
		{
			name: "false condition",
			status: HandlerStatus{
				Type:    "APMReady",
				Status:  metav1.ConditionFalse,
				Reason:  "ValidationFailed",
				Message: "invalid config",
			},
			generation: 1,
			want: metav1.Condition{
				Type:               "APMReady",
				Status:             metav1.ConditionFalse,
				Reason:             "ValidationFailed",
				Message:            "invalid config",
				ObservedGeneration: 1,
			},
		},
		{
			name: "unknown condition",
			status: HandlerStatus{
				Type:    "ChecksReady",
				Status:  metav1.ConditionUnknown,
				Reason:  "NotImplemented",
				Message: "handler not yet implemented",
			},
			generation: 0,
			want: metav1.Condition{
				Type:               "ChecksReady",
				Status:             metav1.ConditionUnknown,
				Reason:             "NotImplemented",
				Message:            "handler not yet implemented",
				ObservedGeneration: 0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := conditionFromHandlerStatus(tt.status, tt.generation)
			assert.Equal(t, tt.want.Type, got.Type)
			assert.Equal(t, tt.want.Status, got.Status)
			assert.Equal(t, tt.want.Reason, got.Reason)
			assert.Equal(t, tt.want.Message, got.Message)
			assert.Equal(t, tt.want.ObservedGeneration, got.ObservedGeneration)
		})
	}
}

func TestUpdateStatusConditions(t *testing.T) {
	scheme := fakeScheme()
	tests := []struct {
		name           string
		client         dynamic.Interface
		cr             *datadoghq.DatadogInstrumentation
		statuses       []HandlerStatus
		wantConditions []metav1.Condition
	}{
		{
			name:   "sets a single condition on the CR",
			client: dynamicfake.NewSimpleDynamicClient(scheme, newTestCR("ddi-1", "default", 1, nil)),
			cr:     newTestCR("ddi-1", "default", 1, nil),
			statuses: []HandlerStatus{
				{Type: "ChecksReady", Status: metav1.ConditionTrue, Reason: "OK", Message: "done"},
			},
			wantConditions: []metav1.Condition{
				{Type: "ChecksReady", Status: metav1.ConditionTrue, Reason: "OK", Message: "done", ObservedGeneration: 1},
			},
		},
		{
			name:   "sets multiple conditions",
			client: dynamicfake.NewSimpleDynamicClient(scheme, newTestCR("ddi-2", "default", 2, nil)),
			cr:     newTestCR("ddi-2", "default", 2, nil),
			statuses: []HandlerStatus{
				{Type: "ChecksReady", Status: metav1.ConditionTrue, Reason: "OK", Message: "checks ok"},
				{Type: "APMReady", Status: metav1.ConditionFalse, Reason: "Err", Message: "apm failed"},
			},
			wantConditions: []metav1.Condition{
				{Type: "ChecksReady", Status: metav1.ConditionTrue, Reason: "OK", Message: "checks ok", ObservedGeneration: 2},
				{Type: "APMReady", Status: metav1.ConditionFalse, Reason: "Err", Message: "apm failed", ObservedGeneration: 2},
			},
		},
		{
			name:   "skips statuses with empty type",
			client: dynamicfake.NewSimpleDynamicClient(scheme, newTestCR("ddi-3", "default", 1, nil)),
			cr:     newTestCR("ddi-3", "default", 1, nil),
			statuses: []HandlerStatus{
				{Type: "", Status: metav1.ConditionTrue, Reason: "OK", Message: "skip"},
				{Type: "ChecksReady", Status: metav1.ConditionTrue, Reason: "OK", Message: "keep"},
			},
			wantConditions: []metav1.Condition{
				{Type: "ChecksReady", Status: metav1.ConditionTrue, Reason: "OK", Message: "keep", ObservedGeneration: 1},
			},
		},
		{
			name:     "nil CR returns nil",
			client:   dynamicfake.NewSimpleDynamicClient(scheme),
			cr:       nil,
			statuses: []HandlerStatus{{Type: "ChecksReady", Status: metav1.ConditionTrue, Reason: "OK", Message: "ok"}},
		},
		{
			name:   "empty statuses returns no error",
			client: dynamicfake.NewSimpleDynamicClient(scheme, newTestCR("ddi-5", "default", 1, nil)),
			cr:     newTestCR("ddi-5", "default", 1, nil),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := updateStatusConditions(t.Context(), tt.client, tt.cr, tt.statuses)
			require.NoError(t, err)

			if tt.wantConditions == nil {
				return
			}

			got, err := tt.client.Resource(DatadogInstrumentationGVR).Namespace(tt.cr.Namespace).Get(t.Context(), tt.cr.Name, metav1.GetOptions{})
			require.NoError(t, err)

			latest := &datadoghq.DatadogInstrumentation{}
			require.NoError(t, UnstructuredIntoDatadogInstrumentation(got, latest))

			require.Len(t, latest.Status.Conditions, len(tt.wantConditions))
			for i, wantCond := range tt.wantConditions {
				gotCond := latest.Status.Conditions[i]
				assert.Equal(t, wantCond.Type, gotCond.Type)
				assert.Equal(t, wantCond.Status, gotCond.Status)
				assert.Equal(t, wantCond.Reason, gotCond.Reason)
				assert.Equal(t, wantCond.Message, gotCond.Message)
				assert.Equal(t, wantCond.ObservedGeneration, gotCond.ObservedGeneration)
			}
		})
	}
}

func TestUpdateStatusConditionsUpdatesExistingCondition(t *testing.T) {
	scheme := fakeScheme()
	cr := newTestCR("ddi-update", "default", 2, nil)
	cr.Status.Conditions = []metav1.Condition{
		{Type: "ChecksReady", Status: metav1.ConditionFalse, Reason: "Pending", Message: "not ready", ObservedGeneration: 1},
	}

	// Seed the fake client with the CR (including its existing conditions via unstructured)
	unstrObj := &unstructured.Unstructured{}
	require.NoError(t, UnstructuredFromDatadogInstrumentation(cr, unstrObj))
	// Ensure GVK is set for the fake client
	unstrObj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   datadoghq.GroupVersion.Group,
		Version: datadoghq.GroupVersion.Version,
		Kind:    "DatadogInstrumentation",
	})

	client := dynamicfake.NewSimpleDynamicClient(scheme, unstrObj)

	statuses := []HandlerStatus{
		{Type: "ChecksReady", Status: metav1.ConditionTrue, Reason: "Configured", Message: "all good"},
	}

	err := updateStatusConditions(t.Context(), client, cr, statuses)
	require.NoError(t, err)

	got, err := client.Resource(DatadogInstrumentationGVR).Namespace("default").Get(t.Context(), "ddi-update", metav1.GetOptions{})
	require.NoError(t, err)

	latest := &datadoghq.DatadogInstrumentation{}
	require.NoError(t, UnstructuredIntoDatadogInstrumentation(got, latest))

	require.Len(t, latest.Status.Conditions, 1)
	cond := latest.Status.Conditions[0]
	assert.Equal(t, "ChecksReady", cond.Type)
	assert.Equal(t, metav1.ConditionTrue, cond.Status)
	assert.Equal(t, "Configured", cond.Reason)
	assert.Equal(t, "all good", cond.Message)
	assert.Equal(t, int64(2), cond.ObservedGeneration)
}
