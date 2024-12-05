// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
)

func TestAddHorizontalAction(t *testing.T) {
	testTime := time.Now()

	// Test no retention, should move back to keep a single action
	var horizontalEventsRetention time.Duration
	horizontalLastActions := []datadoghq.DatadogPodAutoscalerHorizontalAction{
		{
			Time: metav1.Time{Time: testTime.Add(-10 * time.Minute)},
		},
		{
			Time: metav1.Time{Time: testTime.Add(-8 * time.Minute)},
		},
	}
	addedAction1 := &datadoghq.DatadogPodAutoscalerHorizontalAction{
		Time: metav1.Time{Time: testTime},
	}
	horizontalLastActions = addHorizontalAction(testTime, horizontalEventsRetention, horizontalLastActions, addedAction1)
	assert.Equal(t, []datadoghq.DatadogPodAutoscalerHorizontalAction{*addedAction1}, horizontalLastActions)

	// Add another event, should still keep one
	horizontalLastActions = addHorizontalAction(testTime, horizontalEventsRetention, horizontalLastActions, addedAction1)
	assert.Equal(t, []datadoghq.DatadogPodAutoscalerHorizontalAction{*addedAction1}, horizontalLastActions)

	// 15 minutes retention, should keep everything
	horizontalEventsRetention = 15 * time.Minute
	horizontalLastActions = []datadoghq.DatadogPodAutoscalerHorizontalAction{
		{
			Time: metav1.Time{Time: testTime.Add(-10 * time.Minute)},
		},
		{
			Time: metav1.Time{Time: testTime.Add(-8 * time.Minute)},
		},
	}
	// Adding two fake events
	horizontalLastActions = addHorizontalAction(testTime, horizontalEventsRetention, horizontalLastActions, addedAction1)
	horizontalLastActions = addHorizontalAction(testTime, horizontalEventsRetention, horizontalLastActions, addedAction1)
	assert.Equal(t, []datadoghq.DatadogPodAutoscalerHorizontalAction{
		{
			Time: metav1.Time{Time: testTime.Add(-10 * time.Minute)},
		},
		{
			Time: metav1.Time{Time: testTime.Add(-8 * time.Minute)},
		},
		*addedAction1,
		*addedAction1,
	}, horizontalLastActions)

	// Moving time forward, should keep only the last two events
	testTime = testTime.Add(10 * time.Minute)
	addedAction2 := &datadoghq.DatadogPodAutoscalerHorizontalAction{
		Time: metav1.Time{Time: testTime},
	}
	horizontalLastActions = addHorizontalAction(testTime, horizontalEventsRetention, horizontalLastActions, addedAction2)
	assert.Equal(t, []datadoghq.DatadogPodAutoscalerHorizontalAction{
		*addedAction1,
		*addedAction1,
		*addedAction2,
	}, horizontalLastActions)
}

func TestParseCustomConfigurationAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    RecommenderConfiguration
	}{
		{
			name:        "Empty annotations",
			annotations: map[string]string{},
			expected:    RecommenderConfiguration{},
		},
		{
			name: "URL annotation",
			annotations: map[string]string{
				AnnotationsConfigurationKey: "{\"endpoint\": \"localhost:8080/test\"}",
			},
			expected: RecommenderConfiguration{
				Endpoint: "localhost:8080/test",
			},
		},
		{
			name: "Settings annotation",
			annotations: map[string]string{
				AnnotationsConfigurationKey: "{\"endpoint\": \"localhost:8080/test\", \"settings\": {\"key\": \"value\", \"number\": 1, \"bool\": true, \"array\": [1, 2, 3], \"object\": {\"key\": \"value\"}}}",
			},
			expected: RecommenderConfiguration{
				Endpoint: "localhost:8080/test",
				Settings: map[string]any{
					"key":    "value",
					"number": 1.0,
					"bool":   true,
					"array":  []interface{}{1.0, 2.0, 3.0},
					"object": map[string]interface{}{"key": "value"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			customConfiguration := *parseCustomConfigurationAnnotation(tt.annotations)
			assert.Equal(t, tt.expected, customConfiguration)
		})
	}
}
