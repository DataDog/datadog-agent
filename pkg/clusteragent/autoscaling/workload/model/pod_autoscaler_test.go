// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
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

func TestProfileManagedPodAutoscaler(t *testing.T) {
	maxReplicas := int32(10)
	template := &datadoghq.DatadogPodAutoscalerTemplate{
		Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
			{Type: datadoghqcommon.DatadogPodAutoscalerPodResourceObjectiveType},
		},
		Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
			MaxReplicas: &maxReplicas,
		},
	}
	targetRef := autoscalingv2.CrossVersionObjectReference{
		Kind:       "Deployment",
		Name:       "web-app",
		APIVersion: "apps/v1",
	}

	t.Run("NewPodAutoscalerFromProfile", func(t *testing.T) {
		pai := NewPodAutoscalerFromProfile("prod", "web-app-a1b2c3d4", "high-cpu", template, targetRef, "hash1", "")

		assert.Equal(t, "prod", pai.Namespace())
		assert.Equal(t, "web-app-a1b2c3d4", pai.Name())
		assert.Equal(t, "high-cpu", pai.ProfileName())
		assert.True(t, pai.IsProfileManaged())

		spec := pai.Spec()
		require.NotNil(t, spec)
		assert.Equal(t, targetRef, spec.TargetRef)
		assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerLocalOwner, spec.Owner)
		assert.Equal(t, template.Objectives, spec.Objectives)
		assert.Equal(t, template.Constraints, spec.Constraints)
	})

	t.Run("UpdateFromProfile same profile", func(t *testing.T) {
		pai := NewPodAutoscalerFromProfile("prod", "web-app-a1b2c3d4", "high-cpu", template, targetRef, "hash1", "")

		// Simulate some scaling state
		pai.SetCurrentReplicas(5)

		newMaxReplicas := int32(20)
		newTemplate := &datadoghq.DatadogPodAutoscalerTemplate{
			Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
				{Type: datadoghqcommon.DatadogPodAutoscalerContainerResourceObjectiveType},
			},
			Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
				MaxReplicas: &newMaxReplicas,
			},
		}
		pai.UpdateFromProfile("high-cpu", newTemplate, targetRef, "hash2", "")

		assert.Equal(t, "high-cpu", pai.ProfileName())
		assert.True(t, pai.IsProfileManaged())
		assert.Equal(t, newTemplate.Objectives, pai.Spec().Objectives)
		assert.Equal(t, newTemplate.Constraints, pai.Spec().Constraints)
		// Scaling state preserved
		assert.Equal(t, int32(5), *pai.CurrentReplicas())
	})

	t.Run("UpdateFromProfile different profile", func(t *testing.T) {
		pai := NewPodAutoscalerFromProfile("prod", "web-app-a1b2c3d4", "low-cpu", template, targetRef, "hash1", "")

		newTemplate := &datadoghq.DatadogPodAutoscalerTemplate{
			Objectives: []datadoghqcommon.DatadogPodAutoscalerObjective{
				{Type: datadoghqcommon.DatadogPodAutoscalerCustomQueryObjectiveType},
			},
		}
		pai.UpdateFromProfile("high-mem", newTemplate, targetRef, "hash3", "")

		assert.Equal(t, "high-mem", pai.ProfileName())
		assert.True(t, pai.IsProfileManaged())
		assert.Equal(t, newTemplate.Objectives, pai.Spec().Objectives)
	})

	t.Run("IsProfileManaged true/false", func(t *testing.T) {
		// Profile-managed
		pai := NewPodAutoscalerFromProfile("ns", "name", "prof", template, targetRef, "", "")
		assert.True(t, pai.IsProfileManaged())
		assert.Equal(t, "prof", pai.ProfileName())

		// Not profile-managed (regular autoscaler)
		regularPAI := FakePodAutoscalerInternal{
			Namespace: "ns",
			Name:      "regular",
		}.Build()
		assert.False(t, regularPAI.IsProfileManaged())
		assert.Empty(t, regularPAI.ProfileName())
	})

	t.Run("SetProfileName", func(t *testing.T) {
		pai := FakePodAutoscalerInternal{
			Namespace: "ns",
			Name:      "name",
		}.Build()
		assert.False(t, pai.IsProfileManaged())

		pai.SetProfileName("my-profile")
		assert.True(t, pai.IsProfileManaged())
		assert.Equal(t, "my-profile", pai.ProfileName())
	})

	t.Run("BuildDPASpecFromProfile", func(t *testing.T) {
		spec := BuildDPASpecFromProfile(template, targetRef)

		assert.Equal(t, datadoghqcommon.DatadogPodAutoscalerLocalOwner, spec.Owner)
		assert.Equal(t, targetRef, spec.TargetRef)
		assert.Equal(t, template.ApplyPolicy, spec.ApplyPolicy)
		assert.Equal(t, template.Objectives, spec.Objectives)
		assert.Equal(t, template.Fallback, spec.Fallback)
		assert.Equal(t, template.Constraints, spec.Constraints)
		assert.Equal(t, template.Options, spec.Options)
	})

	t.Run("FakePodAutoscalerInternal ProfileName", func(t *testing.T) {
		fake := FakePodAutoscalerInternal{
			Namespace:   "ns",
			Name:        "name",
			ProfileName: "my-profile",
		}
		pai := fake.Build()
		assert.Equal(t, "my-profile", pai.ProfileName())
		assert.True(t, pai.IsProfileManaged())
	})

	t.Run("NewPodAutoscalerFromProfile with burstable annotation sets IsBurstable", func(t *testing.T) {
		pai := NewPodAutoscalerFromProfile("prod", "web-app-a1b2c3d4", "high-cpu", template, targetRef, "hash1", `{"burstable":true}`)
		assert.True(t, pai.IsBurstable())
	})

	t.Run("NewPodAutoscalerFromProfile without burstable annotation does not set IsBurstable", func(t *testing.T) {
		pai := NewPodAutoscalerFromProfile("prod", "web-app-a1b2c3d4", "high-cpu", template, targetRef, "hash1", "")
		assert.False(t, pai.IsBurstable())
	})

	t.Run("UpdateFromProfile burstable annotation toggling", func(t *testing.T) {
		pai := NewPodAutoscalerFromProfile("prod", "web-app-a1b2c3d4", "high-cpu", template, targetRef, "hash1", "")
		assert.False(t, pai.IsBurstable())

		// Enable burstable via annotation
		pai.UpdateFromProfile("high-cpu", template, targetRef, "hash1-v2", `{"burstable":true}`)
		assert.True(t, pai.IsBurstable())

		// Disable burstable by removing annotation
		pai.UpdateFromProfile("high-cpu", template, targetRef, "hash1-v3", "")
		assert.False(t, pai.IsBurstable())
	})

	t.Run("IsBurstable priority matrix", func(t *testing.T) {
		specBurstable := func(v bool) *datadoghq.DatadogPodAutoscalerSpec {
			return &datadoghq.DatadogPodAutoscalerSpec{
				Options: &datadoghqcommon.DatadogPodAutoscalerOptions{Burstable: &v},
			}
		}
		const burstableAnnot = `{"burstable":true}`
		tests := []struct {
			name  string
			spec  *datadoghq.DatadogPodAutoscalerSpec
			annot string
			want  bool
		}{
			{"spec=true wins over annotation", specBurstable(true), burstableAnnot, true},
			{"spec=false wins over annotation", specBurstable(false), burstableAnnot, false},
			{"annotation enables when no spec", nil, burstableAnnot, true},
			{"no spec, no annotation defaults to false", nil, "", false},
			{"spec.Options without Burstable falls back to annotation", &datadoghq.DatadogPodAutoscalerSpec{Options: &datadoghqcommon.DatadogPodAutoscalerOptions{}}, burstableAnnot, true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				pai := FakePodAutoscalerInternal{
					Namespace:            "ns",
					Name:                 "name",
					Spec:                 tt.spec,
					PreviewAnnotationKey: tt.annot,
				}.Build()
				assert.Equal(t, tt.want, pai.IsBurstable())
			})
		}
	})

	t.Run("BuildStatus options.burstable from spec", func(t *testing.T) {
		burstable := true
		pai := FakePodAutoscalerInternal{
			Namespace: "ns",
			Name:      "name",
			Spec: &datadoghq.DatadogPodAutoscalerSpec{
				Options: &datadoghqcommon.DatadogPodAutoscalerOptions{
					Burstable: &burstable,
				},
			},
		}.Build()
		status := pai.BuildStatus(metav1.Now(), nil)
		require.NotNil(t, status.Options)
		require.NotNil(t, status.Options.Burstable)
		assert.True(t, *status.Options.Burstable)
	})

	t.Run("BuildStatus options.burstable=false from spec is reported", func(t *testing.T) {
		burstable := false
		pai := FakePodAutoscalerInternal{
			Namespace: "ns",
			Name:      "name",
			Spec: &datadoghq.DatadogPodAutoscalerSpec{
				Options: &datadoghqcommon.DatadogPodAutoscalerOptions{
					Burstable: &burstable,
				},
			},
		}.Build()
		status := pai.BuildStatus(metav1.Now(), nil)
		require.NotNil(t, status.Options)
		require.NotNil(t, status.Options.Burstable)
		assert.False(t, *status.Options.Burstable)
	})

	t.Run("BuildStatus options.burstable from annotation", func(t *testing.T) {
		pai := FakePodAutoscalerInternal{
			Namespace:            "ns",
			Name:                 "name",
			PreviewAnnotationKey: `{"burstable":true}`,
		}.Build()
		status := pai.BuildStatus(metav1.Now(), nil)
		require.NotNil(t, status.Options)
		require.NotNil(t, status.Options.Burstable)
		assert.True(t, *status.Options.Burstable)
	})

	t.Run("BuildStatus options nil when burstable not set", func(t *testing.T) {
		pai := FakePodAutoscalerInternal{
			Namespace: "ns",
			Name:      "name",
		}.Build()
		status := pai.BuildStatus(metav1.Now(), nil)
		assert.Nil(t, status.Options)
	})
}

func TestContainerResourcesForStatus(t *testing.T) {
	tests := []struct {
		name     string
		input    []datadoghqcommon.DatadogPodAutoscalerContainerResources
		expected []datadoghqcommon.DatadogPodAutoscalerContainerResources
	}{
		{
			name:     "nil input returns nil",
			input:    nil,
			expected: nil,
		},
		{
			name:     "empty input returns empty",
			input:    []datadoghqcommon.DatadogPodAutoscalerContainerResources{},
			expected: []datadoghqcommon.DatadogPodAutoscalerContainerResources{},
		},
		{
			name: "positive limits and requests pass through unchanged",
			input: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
				{
					Name: "app",
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
			expected: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
				{
					Name: "app",
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
		},
		{
			// removeLimitSentinel (-1) is stored in Limits[cpu] in burstable mode to signal
			// "delete this CPU limit from the pod". It must never appear in the DPA status.
			name: "burstable mode: removeLimitSentinel (-1) on CPU limit is filtered from status",
			input: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
				{
					Name: "app",
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("-1"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
			expected: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
				{
					Name: "app",
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
		},
		{
			name: "CPU request passes through in status",
			input: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
				{
					Name: "app",
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("20m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
			expected: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
				{
					Name: "app",
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("20m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
		},
		{
			// Full burstable scenario: CPU limit sentinel removed, memory limits and requests
			// with a CPU request all surface correctly in the status.
			name: "full burstable scenario: CPU limit sentinel filtered, CPU request preserved",
			input: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
				{
					Name: "app",
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("-1"),
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("20m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
			expected: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
				{
					Name: "app",
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("512Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("20m"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
				},
			},
		},
		{
			// Absent requests/limits fields produce nil maps in the output (not empty maps).
			name: "absent limits and requests produce nil maps",
			input: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
				{
					Name: "app",
				},
			},
			expected: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
				{
					Name: "app",
				},
			},
		},
		{
			name: "multiple containers handled independently",
			input: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
				{
					Name: "app",
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("-1"),
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("20m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
				{
					Name: "sidecar",
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("32Mi"),
					},
				},
			},
			expected: []datadoghqcommon.DatadogPodAutoscalerContainerResources{
				{
					Name: "app",
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("256Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("20m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
				{
					Name: "sidecar",
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("200m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("32Mi"),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &VerticalScalingValues{ContainerResources: tt.input}
			got := v.ContainerResourcesForStatus()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestUpdateFromPodAutoscaler(t *testing.T) {
	t.Run("annotation change", func(t *testing.T) {
		dpa := &datadoghq.DatadogPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "dpa", Namespace: "default", Generation: 1},
			Spec:       datadoghq.DatadogPodAutoscalerSpec{Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner},
		}

		pai := NewPodAutoscalerInternal(dpa)
		assert.False(t, pai.IsBurstable())

		// Annotation-only edit (no generation bump): must be picked up immediately.
		dpa.Annotations = map[string]string{PreviewAnnotationKey: `{"burstable":true}`}
		pai.UpdateFromPodAutoscaler(dpa)
		assert.True(t, pai.IsBurstable(), "annotation-only edit must be picked up")

		// A tags annotation-only edit (no generation bump) must refresh the cached upstream CR.
		dpa.Annotations["ad.datadoghq.com/tags"] = `{"team":"foo"}`
		pai.UpdateFromPodAutoscaler(dpa)
		assert.Equal(t, `{"team":"foo"}`, pai.UpstreamCR().Annotations["ad.datadoghq.com/tags"])
	})

	t.Run("status change", func(t *testing.T) {
		dpa := &datadoghq.DatadogPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "dpa", Namespace: "default", Generation: 1},
			Spec:       datadoghq.DatadogPodAutoscalerSpec{Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner},
			Status: datadoghqcommon.DatadogPodAutoscalerStatus{
				Horizontal: &datadoghqcommon.DatadogPodAutoscalerHorizontalStatus{
					Target: &datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{Replicas: 3},
				},
			},
		}

		pai := NewPodAutoscalerInternal(dpa)
		assert.Equal(t, int32(3), pai.UpstreamCR().Status.Horizontal.Target.Replicas)

		// Status-only update (generation unchanged): upstreamCR must reflect the new status.
		dpa.Status.Horizontal.Target.Replicas = 7
		pai.UpdateFromPodAutoscaler(dpa)
		assert.Equal(t, int32(7), pai.UpstreamCR().Status.Horizontal.Target.Replicas, "status-only update must be picked up")
	})
}

// TestSetActiveScalingValues_NilSource_ClearsVertical verifies that a nil verticalActiveSource
// (no backend recommendation yet) sets scalingValues.Vertical to nil instead of self-assigning
// the previously-constrained value.  Self-assigning propagates the burstable sentinel, which
// causes applyVerticalConstraints(burstable=false) to early-return and suppress the rollout.
func TestSetActiveScalingValues_NilSource_ClearsVertical(t *testing.T) {
	// Simulate a sentinel-containing constrained recommendation (from a previous burstable=true cycle).
	constrainedRec := &VerticalScalingValues{
		ResourcesHash: "burstable-hash",
		ContainerResources: []datadoghqcommon.DatadogPodAutoscalerContainerResources{{
			Name:   "app",
			Limits: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("-1")},
		}},
	}

	// mainScalingValues.Vertical is nil (no backend recommendation yet).
	pai := FakePodAutoscalerInternal{
		Namespace: "ns",
		Name:      "dpa",
		// scalingValues carries the sentinel from the previous burstable cycle.
		ScalingValues: ScalingValues{Vertical: constrainedRec},
	}.Build()

	// SetActiveScalingValues with nil verticalActiveSource.
	pai.SetActiveScalingValues(time.Now(), nil, nil)

	// scalingValues.Vertical must be nil — not the sentinel-containing constrained value.
	// sync() will exit early (no recommendation), preventing a phantom rolloutDecisionComplete.
	got := pai.ScalingValues().Vertical
	assert.Nil(t, got,
		"SetActiveScalingValues(nil source) must set scalingValues.Vertical to nil, not "+
			"self-assign the sentinel-containing constrained value; the sentinel would cause "+
			"applyVerticalConstraints(burstable=false) to early-return and suppress the rollout")
}

func BenchmarkUpdateFromPodAutoscaler(b *testing.B) {
	dpa := &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "burner-server",
			Namespace:  "default",
			Generation: 1,
			Labels: map[string]string{
				ProfileLabelKey: "default-profile",
			},
			Annotations: map[string]string{
				PreviewAnnotationKey:           `{"burstable":true}`,
				ProfileTemplateHashAnnotation:  "abc123def456",
				CustomRecommenderAnnotationKey: `{"endpoint":"https://recommender.internal/v1"}`,
			},
		},
		Spec: datadoghq.DatadogPodAutoscalerSpec{
			Owner: datadoghqcommon.DatadogPodAutoscalerLocalOwner,
		},
	}

	pai := NewPodAutoscalerInternal(dpa)

	b.ReportAllocs()
	for b.Loop() {
		pai.UpdateFromPodAutoscaler(dpa)
	}
}
