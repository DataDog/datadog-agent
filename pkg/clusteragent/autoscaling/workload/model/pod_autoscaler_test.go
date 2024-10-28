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
