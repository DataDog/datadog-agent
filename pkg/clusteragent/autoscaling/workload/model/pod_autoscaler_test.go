// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package model

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

func TestAddHorizontalAction(t *testing.T) {
	testTime := time.Now()

	// Test no retention, should move back to keep a single action
	var horizontalEventsRetention time.Duration
	horizontalLastActions := []datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
		{
			Time: metav1.Time{Time: testTime.Add(-10 * time.Minute)},
		},
		{
			Time: metav1.Time{Time: testTime.Add(-8 * time.Minute)},
		},
	}
	addedAction1 := &datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
		Time: metav1.Time{Time: testTime},
	}
	horizontalLastActions = addHorizontalAction(testTime, horizontalEventsRetention, horizontalLastActions, addedAction1)
	assert.Equal(t, []datadoghqcommon.DatadogPodAutoscalerHorizontalAction{*addedAction1}, horizontalLastActions)

	// Add another event, should still keep one
	horizontalLastActions = addHorizontalAction(testTime, horizontalEventsRetention, horizontalLastActions, addedAction1)
	assert.Equal(t, []datadoghqcommon.DatadogPodAutoscalerHorizontalAction{*addedAction1}, horizontalLastActions)

	// 15 minutes retention, should keep everything
	horizontalEventsRetention = 15 * time.Minute
	horizontalLastActions = []datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
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
	assert.Equal(t, []datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
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
	addedAction2 := &datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
		Time: metav1.Time{Time: testTime},
	}
	horizontalLastActions = addHorizontalAction(testTime, horizontalEventsRetention, horizontalLastActions, addedAction2)
	assert.Equal(t, []datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
		*addedAction1,
		*addedAction1,
		*addedAction2,
	}, horizontalLastActions)
}

func TestGetHorizontalEventsRetention(t *testing.T) {
	tests := []struct {
		name                   string
		policy                 *datadoghq.DatadogPodAutoscalerApplyPolicy
		longestLookbackAllowed time.Duration
		expectedRetention      time.Duration
	}{
		{
			name:                   "No policy, no retention",
			policy:                 nil,
			longestLookbackAllowed: 0,
			expectedRetention:      0,
		},
		{
			name:                   "No policy, 15 minutes retention",
			policy:                 nil,
			longestLookbackAllowed: 15 * time.Minute,
			expectedRetention:      0,
		},
		{
			name: "Scale up policy with rules, 30 minutes retention",
			policy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          "Pods",
							PeriodSeconds: 900,
							Value:         2,
						},
					},
				},
			},
			longestLookbackAllowed: 30 * time.Minute,
			expectedRetention:      15 * time.Minute,
		},
		{
			name: "Scale up policy with rules, 10 minutes max retention",
			policy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          "Pods",
							PeriodSeconds: 900,
							Value:         2,
						},
					},
				},
			},
			longestLookbackAllowed: 10 * time.Minute,
			expectedRetention:      10 * time.Minute,
		},
		{
			name: "Scale up and scale down policy with rules, 30 minutes retention",
			policy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          "Pods",
							PeriodSeconds: 900,
							Value:         2,
						},
					},
				},
				ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          "Pods",
							PeriodSeconds: 960,
							Value:         2,
						},
					},
				},
			},
			longestLookbackAllowed: 30 * time.Minute,
			expectedRetention:      16 * time.Minute,
		},
		{
			name: "Scale up and scale down policy with rules, 10 minutes max retention",
			policy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          "Pods",
							PeriodSeconds: 900,
							Value:         2,
						},
					},
				},
				ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          "Pods",
							PeriodSeconds: 960,
							Value:         2,
						},
					},
				},
			},
			longestLookbackAllowed: 10 * time.Minute,
			expectedRetention:      10 * time.Minute,
		},
		{
			name: "Scale up stabilization window 5 minutes",
			policy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          "Pods",
							PeriodSeconds: 180,
							Value:         2,
						},
					},
					StabilizationWindowSeconds: 300,
				},
				ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          "Pods",
							PeriodSeconds: 180,
							Value:         2,
						},
					},
				},
			},
			longestLookbackAllowed: 30 * time.Minute,
			expectedRetention:      5 * time.Minute,
		},
		{
			name: "Scale down stabilization window 5 minutes",
			policy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          "Pods",
							PeriodSeconds: 180,
							Value:         2,
						},
					},
				},
				ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          "Pods",
							PeriodSeconds: 180,
							Value:         2,
						},
					},
					StabilizationWindowSeconds: 300,
				},
			},
			longestLookbackAllowed: 30 * time.Minute,
			expectedRetention:      5 * time.Minute,
		},
		{
			name: "Stabilization, rules, max retention",
			policy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          "Pods",
							PeriodSeconds: 360,
							Value:         2,
						},
					},
					StabilizationWindowSeconds: 300,
				},
				ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          "Pods",
							PeriodSeconds: 420,
							Value:         2,
						},
					},
					StabilizationWindowSeconds: 180,
				},
			},
			longestLookbackAllowed: 30 * time.Minute,
			expectedRetention:      7 * time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			horizontalEventsRetention := getHorizontalEventsRetention(tt.policy, tt.longestLookbackAllowed)
			assert.Equal(t, tt.expectedRetention, horizontalEventsRetention)
		})
	}
}

func TestParseCustomConfigurationAnnotation(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    *RecommenderConfiguration
		err         error
	}{
		{
			name:        "Empty annotations",
			annotations: map[string]string{},
			expected:    nil,
			err:         nil,
		},
		{
			name: "URL annotation",
			annotations: map[string]string{
				CustomRecommenderAnnotationKey: "{\"endpoint\": \"localhost:8080/test\"}",
			},
			expected: &RecommenderConfiguration{
				Endpoint: "localhost:8080/test",
			},
			err: nil,
		},
		{
			name: "Settings annotation",
			annotations: map[string]string{
				CustomRecommenderAnnotationKey: "{\"endpoint\": \"localhost:8080/test\", \"settings\": {\"key\": \"value\", \"number\": 1, \"bool\": true, \"array\": [1, 2, 3], \"object\": {\"key\": \"value\"}}}",
			},
			expected: &RecommenderConfiguration{
				Endpoint: "localhost:8080/test",
				Settings: map[string]any{
					"key":    "value",
					"number": 1.0,
					"bool":   true,
					"array":  []interface{}{1.0, 2.0, 3.0},
					"object": map[string]interface{}{"key": "value"},
				},
			},
			err: nil,
		},
		{
			name: "Unmarshalable annotations",
			annotations: map[string]string{
				CustomRecommenderAnnotationKey: "{\"endpoint: \"localhost:8080/test\",}",
			},
			expected: nil,
			err:      fmt.Errorf("Failed to parse annotations for custom recommender configuration: invalid character 'l' after object key"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			customConfiguration, err := parseCustomConfigurationAnnotation(tt.annotations)
			if tt.err == nil {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, customConfiguration)
			} else {
				assert.EqualError(t, err, tt.err.Error())
			}
		})
	}
}
