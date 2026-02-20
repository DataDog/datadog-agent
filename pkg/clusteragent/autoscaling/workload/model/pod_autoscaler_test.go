// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package model

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"

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

func TestAddRecommendationToHistory(t *testing.T) {
	testTime := time.Now()

	// Test no retention, should move back to keep a single recommendation
	var horizontalRecommendationsRetention time.Duration
	horizontalLastRecommendations := []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
		{
			GeneratedAt: metav1.Time{Time: testTime.Add(-10 * time.Minute)},
		},
		{
			GeneratedAt: metav1.Time{Time: testTime.Add(-8 * time.Minute)},
		},
	}
	addedRecommendation1 := datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
		GeneratedAt: metav1.Time{Time: testTime},
	}
	horizontalLastRecommendations = addRecommendationToHistory(testTime, horizontalRecommendationsRetention, horizontalLastRecommendations, addedRecommendation1)
	assert.Equal(t, []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{addedRecommendation1}, horizontalLastRecommendations)

	// Add another event, should still keep one
	horizontalLastRecommendations = addRecommendationToHistory(testTime, horizontalRecommendationsRetention, horizontalLastRecommendations, addedRecommendation1)
	assert.Equal(t, []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{addedRecommendation1}, horizontalLastRecommendations)

	// 15 minutes retention, should keep everything
	horizontalRecommendationsRetention = 15 * time.Minute
	horizontalLastRecommendations = []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
		{
			GeneratedAt: metav1.Time{Time: testTime.Add(-10 * time.Minute)},
		},
		{
			GeneratedAt: metav1.Time{Time: testTime.Add(-8 * time.Minute)},
		},
	}
	// Adding two fake events
	horizontalLastRecommendations = addRecommendationToHistory(testTime, horizontalRecommendationsRetention, horizontalLastRecommendations, addedRecommendation1)
	horizontalLastRecommendations = addRecommendationToHistory(testTime, horizontalRecommendationsRetention, horizontalLastRecommendations, addedRecommendation1)
	assert.Equal(t, []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
		{
			GeneratedAt: metav1.Time{Time: testTime.Add(-10 * time.Minute)},
		},
		{
			GeneratedAt: metav1.Time{Time: testTime.Add(-8 * time.Minute)},
		},
		addedRecommendation1,
		addedRecommendation1,
	}, horizontalLastRecommendations)

	// Moving time forward, should keep only the last two events
	testTime = testTime.Add(10 * time.Minute)
	addedRecommendation2 := datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
		GeneratedAt: metav1.Time{Time: testTime},
	}
	horizontalLastRecommendations = addRecommendationToHistory(testTime, horizontalRecommendationsRetention, horizontalLastRecommendations, addedRecommendation2)
	assert.Equal(t, []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
		addedRecommendation1,
		addedRecommendation1,
		addedRecommendation2,
	}, horizontalLastRecommendations)
}

func TestGetHorizontalRetentionValues(t *testing.T) {
	tests := []struct {
		name                            string
		policy                          *datadoghq.DatadogPodAutoscalerApplyPolicy
		expectedEventsRetention         time.Duration
		expectedRecommendationRetention time.Duration
	}{
		{
			name:                            "No policy, no retention",
			policy:                          nil,
			expectedEventsRetention:         0,
			expectedRecommendationRetention: 0,
		},
		{
			name: "Scale up policy with rules",
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
			expectedEventsRetention:         15 * time.Minute,
			expectedRecommendationRetention: 0,
		},
		{
			name: "Scale up policy with rules",
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
			expectedEventsRetention:         15 * time.Minute,
			expectedRecommendationRetention: 0,
		},
		{
			name: "Scale up and scale down policy with rules",
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
			expectedEventsRetention:         16 * time.Minute,
			expectedRecommendationRetention: 0,
		},
		{
			name: "Scale up and scale down policy with rules longer than allowed",
			policy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          "Pods",
							PeriodSeconds: 90000,
							Value:         2,
						},
					},
				},
				ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          "Pods",
							PeriodSeconds: 960000,
							Value:         2,
						},
					},
				},
			},
			expectedEventsRetention:         longestScalingRulePeriodAllowed,
			expectedRecommendationRetention: 0,
		},
		{
			name: "Scale up stabilization window",
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
			expectedEventsRetention:         3 * time.Minute,
			expectedRecommendationRetention: 5 * time.Minute,
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
			expectedEventsRetention:         3 * time.Minute,
			expectedRecommendationRetention: 5 * time.Minute,
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
					StabilizationWindowSeconds: 3600,
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
			expectedEventsRetention:         7 * time.Minute,
			expectedRecommendationRetention: 1 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventsRetention, recommendationRetention := getHorizontalRetentionValues(tt.policy)
			assert.Equal(t, tt.expectedEventsRetention, eventsRetention)
			assert.Equal(t, tt.expectedRecommendationRetention, recommendationRetention)
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
			err:      errors.New("Failed to parse annotations for custom recommender configuration: invalid character 'l' after object key"),
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

func TestUpdateFromStatus(t *testing.T) {
	now := time.Now()
	earlierActionTime := now.Add(-10 * time.Minute)
	midActionTime := now.Add(-7 * time.Minute)
	lastRecommendedTime := now.Add(-5 * time.Minute)

	earlierRecommended := int32(4)
	lastRecommended := int32(6)
	limitReason := "some limited reason"
	currentReplicas := int32(7)

	status := &datadoghqcommon.DatadogPodAutoscalerStatus{
		Horizontal: &datadoghqcommon.DatadogPodAutoscalerHorizontalStatus{
			Target: &datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
				Source:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				GeneratedAt: metav1.NewTime(now),
				Replicas:    5,
			},
			LastActions: []datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
				{Time: metav1.NewTime(earlierActionTime), FromReplicas: 2, ToReplicas: 3, RecommendedReplicas: &earlierRecommended},
				{Time: metav1.NewTime(midActionTime), FromReplicas: 3, ToReplicas: 4},
				{Time: metav1.NewTime(lastRecommendedTime), FromReplicas: 4, ToReplicas: 5, RecommendedReplicas: &lastRecommended, LimitedReason: &limitReason},
			},
			LastRecommendations: []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
				{GeneratedAt: metav1.NewTime(earlierActionTime), Replicas: 2},
				{GeneratedAt: metav1.NewTime(midActionTime), Replicas: 3},
				{GeneratedAt: metav1.NewTime(lastRecommendedTime), Replicas: 5},
			},
		},
		Vertical: &datadoghqcommon.DatadogPodAutoscalerVerticalStatus{
			Target: &datadoghqcommon.DatadogPodAutoscalerVerticalTargetStatus{
				Source:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				GeneratedAt: metav1.NewTime(now),
				Version:     "somehash",
				DesiredResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
					{
						Name: "api",
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
					},
				},
			},
			LastAction: &datadoghqcommon.DatadogPodAutoscalerVerticalAction{Time: metav1.NewTime(now), Version: "somehash", Type: datadoghqcommon.DatadogPodAutoscalerRolloutTriggeredVerticalActionType},
		},
		CurrentReplicas: &currentReplicas,
		Conditions: []datadoghqcommon.DatadogPodAutoscalerCondition{
			{Type: datadoghqcommon.DatadogPodAutoscalerErrorCondition, Status: corev1.ConditionTrue, Reason: "globalErr"},
			{Type: datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToRecommendCondition, Status: corev1.ConditionFalse, Reason: "hRecErr"},
			{Type: datadoghqcommon.DatadogPodAutoscalerHorizontalAbleToScaleCondition, Status: corev1.ConditionFalse, Reason: "hScaleErr"},
			{Type: datadoghqcommon.DatadogPodAutoscalerHorizontalScalingLimitedCondition, Status: corev1.ConditionTrue, Reason: limitReason},
			{Type: datadoghqcommon.DatadogPodAutoscalerVerticalAbleToRecommendCondition, Status: corev1.ConditionFalse, Reason: "vRecErr"},
			{Type: datadoghqcommon.DatadogPodAutoscalerVerticalAbleToApply, Status: corev1.ConditionFalse, Reason: "vApplyErr"},
			{Type: datadoghqcommon.DatadogPodAutoscalerVerticalScalingLimitedCondition, Status: corev1.ConditionTrue, Reason: "LimitedByConstraint", Message: "clamped for containers: app"},
		},
	}

	var actual PodAutoscalerInternal
	actual.horizontalRecommendationsRetention = 10 * time.Minute
	actual.UpdateFromStatus(status)

	expected := FakePodAutoscalerInternal{
		ScalingValues: ScalingValues{
			Horizontal: &HorizontalScalingValues{
				Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp: now,
				Replicas:  5,
			},
			Vertical: &VerticalScalingValues{
				Source:        datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp:     now,
				ResourcesHash: "somehash",
				ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
					{
						Name: "api", Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("100Mi"),
						},
					},
				},
			},
			HorizontalError: errors.New("hRecErr"),
			VerticalError:   errors.New("vRecErr"),
		},
		HorizontalLastActions:              status.Horizontal.LastActions,
		HorizontalRecommendationsRetention: 10 * time.Minute,
		HorizontalLastRecommendations: []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
			{GeneratedAt: metav1.NewTime(earlierActionTime), Replicas: 2},
			{GeneratedAt: metav1.NewTime(midActionTime), Replicas: 3},
			{GeneratedAt: metav1.NewTime(lastRecommendedTime), Replicas: 5},
		},
		HorizontalLastLimitReason: limitReason,
		HorizontalLastActionError: errors.New("hScaleErr"),
		VerticalLastAction:        status.Vertical.LastAction,
		VerticalLastActionError:   errors.New("vApplyErr"),
		VerticalLastLimitReason:   autoscaling.NewConditionError(autoscaling.ConditionReasonLimitedByConstraint, errors.New("clamped for containers: app")),
		CurrentReplicas:           &currentReplicas,
		Error:                     errors.New("globalErr"),
	}

	AssertPodAutoscalersEqual(t, expected, actual)
}
