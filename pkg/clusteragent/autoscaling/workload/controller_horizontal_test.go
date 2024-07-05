// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	v2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	clock "k8s.io/utils/clock/testing"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

type horizontalControllerFixture struct {
	t          *testing.T
	clock      *clock.FakeClock
	recorder   *record.FakeRecorder
	scaler     *fakeScaler
	controller *horizontalController
}

func newHorizontalControllerFixture(t *testing.T, testTime time.Time) *horizontalControllerFixture {
	clock := clock.NewFakeClock(testTime)
	recorder := record.NewFakeRecorder(100)
	scaler := newFakeScaler()

	return &horizontalControllerFixture{
		t:        t,
		clock:    clock,
		recorder: recorder,
		scaler:   scaler,

		controller: &horizontalController{
			clock:         clock,
			eventRecorder: recorder,
			scaler:        scaler,
		},
	}
}

func (f *horizontalControllerFixture) resetFakeScaler() {
	f.scaler = newFakeScaler()
	f.controller.scaler = f.scaler
}

func (f *horizontalControllerFixture) runSync(fakePai *model.FakePodAutoscalerInternal) (model.PodAutoscalerInternal, autoscaling.ProcessResult, error) {
	// PodAutoscaler object is only used to attach events to the fake recorder
	// The controller does not interact with the object itself
	fakeAutoscaler := &datadoghq.DatadogPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fakePai.Name,
			Namespace: fakePai.Namespace,
		},
	}

	autoscalerInternal := fakePai.Build()
	res, err := f.controller.sync(context.Background(), fakeAutoscaler, &autoscalerInternal)
	return autoscalerInternal, res, err
}

type horizontalScalingTestArgs struct {
	fakePai          *model.FakePodAutoscalerInternal
	dataSource       datadoghq.DatadogPodAutoscalerValueSource
	dataOffset       time.Duration
	currentReplicas  int32
	statusReplicas   int32
	recReplicas      int32
	scaleReplicas    int32
	scaleError       error
	scaleLimitReason string
}

func (f *horizontalControllerFixture) testScalingDecision(args horizontalScalingTestArgs) (autoscaling.ProcessResult, error) {
	f.t.Helper()
	f.resetFakeScaler()
	f.scaler.mockGet(*args.fakePai, args.currentReplicas, args.statusReplicas, nil)

	scaleActionExpected := args.currentReplicas != args.scaleReplicas
	expectedUpdateCalls := 0
	if scaleActionExpected {
		f.scaler.mockUpdate(*args.fakePai, args.scaleReplicas, args.statusReplicas, args.scaleError)
		expectedUpdateCalls = 1
	}

	args.fakePai.ScalingValues.Horizontal = &model.HorizontalScalingValues{
		Source:    args.dataSource,
		Timestamp: f.clock.Now().Add(-args.dataOffset),
		Replicas:  args.recReplicas,
	}

	autoscaler, result, err := f.runSync(args.fakePai)
	f.scaler.AssertNumberOfCalls(f.t, "get", 1)
	f.scaler.AssertNumberOfCalls(f.t, "update", expectedUpdateCalls)

	args.fakePai.CurrentReplicas = pointer.Ptr[int32](args.statusReplicas)
	if scaleActionExpected && args.scaleError == nil {
		// Update fakePai with the new expected state
		action := &datadoghq.DatadogPodAutoscalerHorizontalAction{
			Time:                metav1.NewTime(f.clock.Now()),
			FromReplicas:        args.currentReplicas,
			ToReplicas:          args.scaleReplicas,
			RecommendedReplicas: pointer.Ptr[int32](args.recReplicas),
		}
		if args.scaleLimitReason != "" {
			action.LimitedReason = &args.scaleLimitReason
		}

		args.fakePai.AddHorizontalAction(action.Time.Time, action)
		args.fakePai.HorizontalLastActionError = nil
	} else if args.scaleError != nil {
		args.fakePai.HorizontalLastActionError = args.scaleError
	}

	args.fakePai.HorizontalLastLimitReason = args.scaleLimitReason

	model.AssertPodAutoscalersEqual(f.t, *args.fakePai, autoscaler)
	return result, err
}

func TestHorizontalControllerSyncPrerequisites(t *testing.T) {
	f := newHorizontalControllerFixture(t, time.Now())
	autoscalerNamespace := "default"
	autoscalerName := "test"

	fakePai := &model.FakePodAutoscalerInternal{
		Namespace: autoscalerNamespace,
		Name:      autoscalerName,
	}

	// Test case: no Spec, no action taken
	autoscaler, result, err := f.runSync(fakePai)
	assert.Equal(t, result, autoscaling.NoRequeue)
	assert.Nil(t, err)
	model.AssertPodAutoscalersEqual(t, fakePai.Build(), autoscaler)

	// Test case: Spec has been added, but no GVK
	fakePai.Spec = &datadoghq.DatadogPodAutoscalerSpec{
		TargetRef: v2.CrossVersionObjectReference{
			Name: "test",
		},
	}
	autoscaler, result, err = f.runSync(fakePai)
	assert.Equal(t, result, autoscaling.NoRequeue)
	assert.EqualError(t, err, "failed to parse API version '', err: %!w(<nil>)")
	fakePai.Error = testutil.NewErrorString("failed to parse API version '', err: %!w(<nil>)")
	model.AssertPodAutoscalersEqual(t, fakePai.Build(), autoscaler)

	// Test case: Correct Spec and GVK, but no scaling values
	// Should only update replica count
	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}
	targetGR := schema.GroupResource{
		Group:    expectedGVK.Group,
		Resource: expectedGVK.Kind,
	}
	fakePai.Spec = &datadoghq.DatadogPodAutoscalerSpec{
		TargetRef: v2.CrossVersionObjectReference{
			Name:       autoscalerName,
			Kind:       expectedGVK.Kind,
			APIVersion: expectedGVK.Group + "/" + expectedGVK.Version,
		},
	}
	fakePai.Error = nil
	f.scaler.On("get", mock.Anything, autoscalerNamespace, autoscalerName, expectedGVK).Return(
		&autoscalingv1.Scale{
			Spec: autoscalingv1.ScaleSpec{
				Replicas: 5,
			},
			Status: autoscalingv1.ScaleStatus{
				Replicas: 5,
			},
		},
		targetGR,
		nil,
	)
	autoscaler, result, err = f.runSync(fakePai)
	f.scaler.AssertNumberOfCalls(t, "get", 1)
	assert.Equal(t, result, autoscaling.NoRequeue)
	assert.NoError(t, err)
	// Update fakePai with the new expected state
	fakePai = &model.FakePodAutoscalerInternal{
		Namespace:       autoscalerNamespace,
		Name:            autoscalerName,
		Spec:            fakePai.Spec,
		CurrentReplicas: pointer.Ptr[int32](5),
		TargetGVK:       expectedGVK,
	}
	model.AssertPodAutoscalersEqual(t, *fakePai, autoscaler)

	// Test case: Error returned while getting scale subresource
	f.resetFakeScaler()
	f.scaler.On("get", mock.Anything, autoscalerNamespace, autoscalerName, expectedGVK).Return(&autoscalingv1.Scale{}, schema.GroupResource{}, testutil.NewErrorString("some k8s error"))
	autoscaler, result, err = f.runSync(fakePai)
	f.scaler.AssertNumberOfCalls(t, "get", 1)
	assert.Equal(t, result, autoscaling.Requeue)
	assert.EqualError(t, err, "failed to get scale subresource for autoscaler default/test, err: some k8s error")
	model.AssertPodAutoscalersEqual(t, model.FakePodAutoscalerInternal{
		Namespace:                 autoscalerNamespace,
		Name:                      autoscalerName,
		Spec:                      fakePai.Spec,
		CurrentReplicas:           pointer.Ptr[int32](5),
		TargetGVK:                 expectedGVK,
		HorizontalLastActionError: testutil.NewErrorString("failed to get scale subresource for autoscaler default/test, err: some k8s error"),
	}, autoscaler)

	// Test case: Any scaling disabled by policy
	fakePai.Spec.Policy = &datadoghq.DatadogPodAutoscalerPolicy{
		ApplyMode: datadoghq.DatadogPodAutoscalerNoneApplyMode,
	}
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		currentReplicas: 5,
		statusReplicas:  5,
		recReplicas:     10,
		scaleReplicas:   5,
		scaleError:      testutil.NewErrorString("horizontal scaling disabled due to applyMode: None not allowing recommendations from source: Autoscaling"),
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test case: Automatic scaling values disabled by policy
	fakePai.Spec.Policy = &datadoghq.DatadogPodAutoscalerPolicy{
		ApplyMode: datadoghq.DatadogPodAutoscalerManualApplyMode,
	}
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		currentReplicas: 5,
		statusReplicas:  5,
		recReplicas:     10,
		scaleReplicas:   5,
		scaleError:      testutil.NewErrorString("horizontal scaling disabled due to applyMode: Manual not allowing recommendations from source: Autoscaling"),
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test case: Automatic scaling values disabled by policy, sending manual value, should scale
	fakePai.Spec.Policy = &datadoghq.DatadogPodAutoscalerPolicy{
		ApplyMode: datadoghq.DatadogPodAutoscalerManualApplyMode,
	}
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerManualValueSource,
		currentReplicas: 5,
		statusReplicas:  5,
		recReplicas:     10,
		scaleReplicas:   10,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)
}

func TestHorizontalControllerSyncScaleDecisions(t *testing.T) {
	testTime := time.Now()
	startTime := testTime.Add(-time.Hour)
	defaultStepDuration := 30 * time.Second

	f := newHorizontalControllerFixture(t, startTime)
	autoscalerNamespace := "default"
	autoscalerName := "test"

	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}
	fakePai := &model.FakePodAutoscalerInternal{
		Namespace: autoscalerNamespace,
		Name:      autoscalerName,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name:       autoscalerName,
				Kind:       expectedGVK.Kind,
				APIVersion: expectedGVK.Group + "/" + expectedGVK.Version,
			},
		},
		ScalingValues: model.ScalingValues{
			Horizontal: &model.HorizontalScalingValues{
				Source:    datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp: f.clock.Now().Add(-defaultStepDuration),
				Replicas:  5,
			},
		},
		TargetGVK: expectedGVK,
	}

	// Step: same number of replicas, no action taken, only updating status
	result, err := f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 5,
		statusReplicas:  4,
		recReplicas:     5,
		scaleReplicas:   5,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Step: Received increased replicas, should scale up
	// Step is 30s later
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 5,
		statusReplicas:  4,
		recReplicas:     7,
		scaleReplicas:   7,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Step: Received decreased replicas, should scale down
	// Step is 30s later
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 7,
		statusReplicas:  4,
		recReplicas:     5,
		scaleReplicas:   5,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Step: Error while scaling down to 3 replicas
	// Step is 30s later
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 5,
		statusReplicas:  4,
		recReplicas:     3,
		scaleReplicas:   3,
		scaleError:      testutil.NewErrorString("some k8s error"),
	})
	assert.Equal(t, autoscaling.Requeue, result)
	assert.EqualError(t, err, "failed to scale target: default/test to 3 replicas, err: some k8s error")

	// Step:: No error, scaling resumed
	// Step is 30s later
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 5,
		statusReplicas:  5,
		recReplicas:     10,
		scaleReplicas:   10,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Step: Changing Min/Max replicas in Spec, current replicas outside of new range (max)
	// Scaling should be limited by the new constraints
	// Step is 30s later
	f.clock.Step(defaultStepDuration)
	fakePai.Spec.Constraints = &datadoghq.DatadogPodAutoscalerConstraints{
		MinReplicas: pointer.Ptr[int32](2),
		MaxReplicas: 8,
	}
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  10,
		statusReplicas:   10,
		recReplicas:      10,
		scaleReplicas:    8,
		scaleLimitReason: "current replica count is outside of min/max constraints, scaling back to closest boundary: 8 replicas",
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Step: Changes in recommended replicas but still outside of new range (max)
	// Step is 30s later
	f.clock.Step(defaultStepDuration)
	fakePai.Spec.Constraints = &datadoghq.DatadogPodAutoscalerConstraints{
		MinReplicas: pointer.Ptr[int32](2),
		MaxReplicas: 8,
	}
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  8,
		statusReplicas:   8,
		recReplicas:      9,
		scaleReplicas:    8,
		scaleLimitReason: "desired replica count limited to 8 (originally 9) due to max replicas constraint",
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Step: Changing Min/Max replicas in Spec, current replicas outside of new range (min)
	// Scaling should be limited by the new constraints
	// Step is 30s later
	f.clock.Step(defaultStepDuration)
	fakePai.Spec.Constraints = &datadoghq.DatadogPodAutoscalerConstraints{
		MinReplicas: pointer.Ptr[int32](8),
		MaxReplicas: 10,
	}
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  6,
		statusReplicas:   6,
		recReplicas:      7,
		scaleReplicas:    8,
		scaleLimitReason: "current replica count is outside of min/max constraints, scaling back to closest boundary: 8 replicas",
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Step: Changes in recommended replicas but still outside of new range (min)
	// Step is 30s later
	f.clock.Step(defaultStepDuration)
	fakePai.Spec.Constraints = &datadoghq.DatadogPodAutoscalerConstraints{
		MinReplicas: pointer.Ptr[int32](8),
		MaxReplicas: 10,
	}
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  8,
		statusReplicas:   8,
		recReplicas:      7,
		scaleReplicas:    8,
		scaleLimitReason: "desired replica count limited to 8 (originally 7) due to min replicas constraint",
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)
}

func TestHorizontalControllerSyncUpscaleWithRules(t *testing.T) {
	testTime := time.Now()
	startTime := testTime.Add(-time.Hour)
	defaultStepDuration := 30 * time.Second

	f := newHorizontalControllerFixture(t, startTime)
	autoscalerNamespace := "default"
	autoscalerName := "test"

	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}
	// Rules used on Upscale
	// Min of
	// - 5 PODs every 1 minute
	// - 20% change every 5 minutes
	// - 100% change every 30 minutes
	//
	// Start at 100 replicas for ease of calculations
	fakePai := &model.FakePodAutoscalerInternal{
		Namespace: autoscalerNamespace,
		Name:      autoscalerName,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name:       autoscalerName,
				Kind:       expectedGVK.Kind,
				APIVersion: expectedGVK.Group + "/" + expectedGVK.Version,
			},
			Policy: &datadoghq.DatadogPodAutoscalerPolicy{
				Upscale: &datadoghq.DatadogPodAutoscalerScalingPolicy{
					Strategy: pointer.Ptr(datadoghq.DatadogPodAutoscalerMinChangeStrategySelect),
					Rules: []datadoghq.DatadogPodAutoscalerScalingRule{
						{
							Type:          datadoghq.DatadogPodAutoscalerPodsScalingRuleType,
							Value:         5,
							PeriodSeconds: 60,
						},
						{
							Type:          datadoghq.DatadogPodAutoscalerPercentScalingRuleType,
							Value:         20,
							PeriodSeconds: 300,
						},
						{
							Type:          datadoghq.DatadogPodAutoscalerPercentScalingRuleType,
							Value:         100,
							PeriodSeconds: 1800,
						},
					},
				},
			},
		},
		ScalingValues: model.ScalingValues{
			Horizontal: &model.HorizontalScalingValues{
				Source:    datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp: f.clock.Now().Add(-defaultStepDuration),
				Replicas:  100,
			},
		},
		TargetGVK:                 expectedGVK,
		HorizontalEventsRetention: 30 * time.Minute, // Matching rules
	}

	// Step: Increase of 4 replicas in 30s, should scale up without limits
	result, err := f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 100,
		statusReplicas:  100,
		recReplicas:     104,
		scaleReplicas:   104,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Step: Another increase of 4 replicas 30s later (so 8 in 1min), should be blocked to 5 replicas due to first rule
	// Earliest event expiration will be in 30s, so we should requeue after 31s
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  104,
		statusReplicas:   104,
		recReplicas:      108,
		scaleReplicas:    105,
		scaleLimitReason: "desired replica count limited to 105 (originally 108) due to scaling policy",
	})
	assert.Equal(t, autoscaling.Requeue.After(31*time.Second), result)
	assert.NoError(t, err)

	// Step: Scaling up to 125 replicas, we should be maxed out at 120 replicas due to the 20% rule over 5m
	// It goes 105 -> 109 -> 110 -> 114 -> 115 -> 119 -> 120 (6 steps)
	args := horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 104,
		statusReplicas:  104,
		recReplicas:     125,
		scaleReplicas:   105,
	}

	for i := 0; i < 6; i++ {
		f.clock.Step(defaultStepDuration)

		args.currentReplicas = args.scaleReplicas
		args.statusReplicas = args.scaleReplicas

		var addedReplicas int32
		if i%2 == 0 {
			addedReplicas = 4
		} else {
			addedReplicas = 1
		}
		args.scaleReplicas += addedReplicas
		args.scaleLimitReason = fmt.Sprintf("desired replica count limited to %d (originally 125) due to scaling policy", args.scaleReplicas)

		result, err = f.testScalingDecision(args)
		assert.Equal(t, autoscaling.Requeue.After(31*time.Second), result)
		assert.NoError(t, err)
	}

	// Now we can check that we are limited to 120 replicas
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  120,
		statusReplicas:   120,
		recReplicas:      125,
		scaleReplicas:    120,
		scaleLimitReason: "desired replica count limited to 120 (originally 125) due to scaling policy",
	})
	assert.Equal(t, autoscaling.Requeue.After(31*time.Second), result)
	assert.NoError(t, err)

	// Step: To break the 100% limit we cheat and add a scaling event that should have been forbidden by the other rules
	f.clock.Step(defaultStepDuration)
	fakePai.HorizontalLastActions = append(fakePai.HorizontalLastActions, datadoghq.DatadogPodAutoscalerHorizontalAction{
		Time:                metav1.NewTime(f.clock.Now()),
		FromReplicas:        120,
		ToReplicas:          198,
		RecommendedReplicas: pointer.Ptr[int32](198),
	})

	// Moving clock 6 minutes forward to only block on 100% rule
	f.clock.Step(6 * time.Minute)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  198,
		statusReplicas:   198,
		recReplicas:      202,
		scaleReplicas:    200,
		scaleLimitReason: "desired replica count limited to 200 (originally 202) due to scaling policy",
	})
	assert.Equal(t, autoscaling.Requeue.After(61*time.Second), result)
	assert.NoError(t, err)

	// Changing strategy from MinChange to MaxChange, the 20% rule should now be the limiting factor (as we moved 6 minutes since large increase)
	// 20% of 198 leads to 238
	fakePai.Spec.Policy.Upscale.Strategy = pointer.Ptr(datadoghq.DatadogPodAutoscalerMaxChangeStrategySelect)
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  200,
		statusReplicas:   200,
		recReplicas:      250,
		scaleReplicas:    238,
		scaleLimitReason: "desired replica count limited to 238 (originally 250) due to scaling policy",
	})
	assert.Equal(t, autoscaling.Requeue.After(31*time.Second), result)
	assert.NoError(t, err)

	// Step: Add a rule IfScalingEvent to block scaling up during 1 minute after an event
	f.clock.Step(time.Hour)
	fakePai.Spec.Policy.Upscale.Strategy = pointer.Ptr(datadoghq.DatadogPodAutoscalerMinChangeStrategySelect)
	fakePai.Spec.Policy.Upscale.Rules = append(fakePai.Spec.Policy.Upscale.Rules, datadoghq.DatadogPodAutoscalerScalingRule{
		Type:          datadoghq.DatadogPodAutoscalerPodsScalingRuleType,
		Value:         0,
		PeriodSeconds: 60,
		Match:         pointer.Ptr(datadoghq.DatadogPodAutoscalerIfScalingEventRuleMatch),
	})

	// Scaling allowed as no event occurred recently
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 238,
		statusReplicas:  238,
		recReplicas:     240,
		scaleReplicas:   240,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Scaling not allowed by the 0-pod rule / 60s rule
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  240,
		statusReplicas:   240,
		recReplicas:      245,
		scaleReplicas:    240,
		scaleLimitReason: "desired replica count limited to 240 (originally 245) due to scaling policy",
	})
	assert.Equal(t, autoscaling.Requeue.After(31*time.Second), result)
	assert.NoError(t, err)

	// After 1 minute we should be able to scale again
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 240,
		statusReplicas:  240,
		recReplicas:     245,
		scaleReplicas:   245,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)
}

func TestHorizontalControllerSyncDownscaleWithRules(t *testing.T) {
	testTime := time.Now()
	startTime := testTime.Add(-time.Hour)
	defaultStepDuration := 30 * time.Second

	f := newHorizontalControllerFixture(t, startTime)
	autoscalerNamespace := "default"
	autoscalerName := "test"

	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}
	// Rules used on Downscale
	// Min of
	// - 5 PODs every 1 minute
	// - 20% change every 5 minutes
	// - 100% change every 30 minutes
	//
	// Start at 100 replicas for ease of calculations
	fakePai := &model.FakePodAutoscalerInternal{
		Namespace: autoscalerNamespace,
		Name:      autoscalerName,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name:       autoscalerName,
				Kind:       expectedGVK.Kind,
				APIVersion: expectedGVK.Group + "/" + expectedGVK.Version,
			},
			Policy: &datadoghq.DatadogPodAutoscalerPolicy{
				Downscale: &datadoghq.DatadogPodAutoscalerScalingPolicy{
					Strategy: pointer.Ptr(datadoghq.DatadogPodAutoscalerMinChangeStrategySelect),
					Rules: []datadoghq.DatadogPodAutoscalerScalingRule{
						{
							Type:          datadoghq.DatadogPodAutoscalerPodsScalingRuleType,
							Value:         5,
							PeriodSeconds: 60,
						},
						{
							Type:          datadoghq.DatadogPodAutoscalerPercentScalingRuleType,
							Value:         20,
							PeriodSeconds: 300,
						},
						{
							Type:          datadoghq.DatadogPodAutoscalerPercentScalingRuleType,
							Value:         50,
							PeriodSeconds: 1800,
						},
					},
				},
			},
		},
		ScalingValues: model.ScalingValues{
			Horizontal: &model.HorizontalScalingValues{
				Source:    datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp: f.clock.Now().Add(-defaultStepDuration),
				Replicas:  100,
			},
		},
		TargetGVK:                 expectedGVK,
		HorizontalEventsRetention: 30 * time.Minute, // Matching rules
	}

	// Step: Decrease of 4 replicas in 30s, should scale down without limits
	result, err := f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 100,
		statusReplicas:  100,
		recReplicas:     96,
		scaleReplicas:   96,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Step: Another decrease of 4 replicas 30s later (so 8 in 1min), should be blocked to 5 replicas due to first rule
	// Earliest event expiration will be in 30s, so we should requeue after 31s
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  96,
		statusReplicas:   96,
		recReplicas:      92,
		scaleReplicas:    95,
		scaleLimitReason: "desired replica count limited to 95 (originally 92) due to scaling policy",
	})
	assert.Equal(t, autoscaling.Requeue.After(31*time.Second), result)
	assert.NoError(t, err)

	// Step: Scaling up to 125 replicas, we should be maxed out at 120 replicas due to the 20% rule over 5m
	// It goes 95 -> 91 -> 90 -> 86 -> 85 -> 81 -> 80 (6 steps)
	args := horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 96,
		statusReplicas:  96,
		recReplicas:     75,
		scaleReplicas:   95,
	}

	for i := 0; i < 6; i++ {
		f.clock.Step(defaultStepDuration)

		args.currentReplicas = args.scaleReplicas
		args.statusReplicas = args.scaleReplicas

		var deletedReplicas int32
		if i%2 == 0 {
			deletedReplicas = 4
		} else {
			deletedReplicas = 1
		}
		args.scaleReplicas -= deletedReplicas
		args.scaleLimitReason = fmt.Sprintf("desired replica count limited to %d (originally 75) due to scaling policy", args.scaleReplicas)

		result, err = f.testScalingDecision(args)
		assert.Equal(t, autoscaling.Requeue.After(31*time.Second), result)
		assert.NoError(t, err)
	}

	// Now we can check that we are limited to 80 replicas
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  80,
		statusReplicas:   80,
		recReplicas:      75,
		scaleReplicas:    80,
		scaleLimitReason: "desired replica count limited to 80 (originally 75) due to scaling policy",
	})
	assert.Equal(t, autoscaling.Requeue.After(31*time.Second), result)
	assert.NoError(t, err)

	// Step: To break the 50% limit we cheat and add a scaling event that should have been forbidden by the other rules
	f.clock.Step(defaultStepDuration)
	fakePai.HorizontalLastActions = append(fakePai.HorizontalLastActions, datadoghq.DatadogPodAutoscalerHorizontalAction{
		Time:                metav1.NewTime(f.clock.Now()),
		FromReplicas:        80,
		ToReplicas:          52,
		RecommendedReplicas: pointer.Ptr[int32](52),
	})

	// Moving clock 6 minutes forward to only block on 50% rule
	f.clock.Step(6 * time.Minute)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  52,
		statusReplicas:   52,
		recReplicas:      48,
		scaleReplicas:    50,
		scaleLimitReason: "desired replica count limited to 50 (originally 48) due to scaling policy",
	})
	assert.Equal(t, autoscaling.Requeue.After(61*time.Second), result)
	assert.NoError(t, err)

	// Changing strategy from MinChange to MaxChange, the 20% rule should now be the limiting factor (as we moved 6 minutes since large decrease)
	// 20% of 52 leads to 41.6 (41)
	fakePai.Spec.Policy.Downscale.Strategy = pointer.Ptr(datadoghq.DatadogPodAutoscalerMaxChangeStrategySelect)
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  50,
		statusReplicas:   50,
		recReplicas:      40,
		scaleReplicas:    41,
		scaleLimitReason: "desired replica count limited to 41 (originally 40) due to scaling policy",
	})
	assert.Equal(t, autoscaling.Requeue.After(31*time.Second), result)
	assert.NoError(t, err)

	// Step: Add a rule IfScalingEvent to block scaling up during 1 minute after an event
	f.clock.Step(time.Hour)
	fakePai.Spec.Policy.Downscale.Strategy = pointer.Ptr(datadoghq.DatadogPodAutoscalerMinChangeStrategySelect)
	fakePai.Spec.Policy.Downscale.Rules = append(fakePai.Spec.Policy.Downscale.Rules, datadoghq.DatadogPodAutoscalerScalingRule{
		Type:          datadoghq.DatadogPodAutoscalerPodsScalingRuleType,
		Value:         0,
		PeriodSeconds: 60,
		Match:         pointer.Ptr(datadoghq.DatadogPodAutoscalerIfScalingEventRuleMatch),
	})

	// Scaling allowed as no event occurred recently
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 41,
		statusReplicas:  41,
		recReplicas:     40,
		scaleReplicas:   40,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Scaling not allowed by the 0-pod rule / 60s rule
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  40,
		statusReplicas:   40,
		recReplicas:      35,
		scaleReplicas:    40,
		scaleLimitReason: "desired replica count limited to 40 (originally 35) due to scaling policy",
	})
	assert.Equal(t, autoscaling.Requeue.After(31*time.Second), result)
	assert.NoError(t, err)

	// After 1 minute we should be able to scale again
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 40,
		statusReplicas:  40,
		recReplicas:     35,
		scaleReplicas:   35,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)
}

func TestHorizontalControllerSyncScaleDecisionsWithRules(t *testing.T) {
	testTime := time.Now()
	startTime := testTime.Add(-time.Hour)
	defaultStepDuration := 30 * time.Second

	f := newHorizontalControllerFixture(t, startTime)
	autoscalerNamespace := "default"
	autoscalerName := "test"

	expectedGVK := schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}

	// Single rule on upscale and downscale
	// Start at 100 replicas for ease of calculations
	fakePai := &model.FakePodAutoscalerInternal{
		Namespace: autoscalerNamespace,
		Name:      autoscalerName,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name:       autoscalerName,
				Kind:       expectedGVK.Kind,
				APIVersion: expectedGVK.Group + "/" + expectedGVK.Version,
			},
			Constraints: &datadoghq.DatadogPodAutoscalerConstraints{
				MinReplicas: pointer.Ptr[int32](90),
				MaxReplicas: 120,
			},
			Policy: &datadoghq.DatadogPodAutoscalerPolicy{
				Upscale: &datadoghq.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghq.DatadogPodAutoscalerScalingRule{
						{
							Type:          datadoghq.DatadogPodAutoscalerPodsScalingRuleType,
							Value:         5,
							PeriodSeconds: 300,
						},
					},
				},
				Downscale: &datadoghq.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghq.DatadogPodAutoscalerScalingRule{
						{
							Type:          datadoghq.DatadogPodAutoscalerPodsScalingRuleType,
							Value:         5,
							PeriodSeconds: 300,
						},
					},
				},
			},
		},
		ScalingValues: model.ScalingValues{
			Horizontal: &model.HorizontalScalingValues{
				Source:    datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp: f.clock.Now().Add(-defaultStepDuration),
				Replicas:  100,
			},
		},
		TargetGVK:                 expectedGVK,
		HorizontalEventsRetention: 5 * time.Minute, // Matching rules
	}

	// Test upscale to 103 replicas (not limited)
	f.clock.Step(defaultStepDuration)
	result, err := f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 100,
		statusReplicas:  100,
		recReplicas:     103,
		scaleReplicas:   103,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test downscale to 97 replicas (not limited)
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 103,
		statusReplicas:  103,
		recReplicas:     97,
		scaleReplicas:   97,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test downscale to 92 replicas, limited to 95
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  97,
		statusReplicas:   97,
		recReplicas:      92,
		scaleReplicas:    95,
		scaleLimitReason: "desired replica count limited to 95 (originally 92) due to scaling policy",
	})
	assert.Equal(t, autoscaling.Requeue.After(241*time.Second), result)
	assert.NoError(t, err)

	// Test upscale to 110 replicas, limited to 105
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  95,
		statusReplicas:   95,
		recReplicas:      110,
		scaleReplicas:    105,
		scaleLimitReason: "desired replica count limited to 105 (originally 110) due to scaling policy",
	})
	assert.Equal(t, autoscaling.Requeue.After(211*time.Second), result)
	assert.NoError(t, err)

	// Test out of range current replicas of 130, limited to 120
	// Moving clock 10 minutes forward to avoid the 5 pods rule
	f.clock.Step(10 * time.Minute)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  130,
		statusReplicas:   130,
		recReplicas:      140,
		scaleReplicas:    120,
		scaleLimitReason: "current replica count is outside of min/max constraints, scaling back to closest boundary: 120 replicas",
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Setting Upscaling strategy to Disabled, should only allow downscaling
	// Moving clock 10 minutes forward to avoid the 5 pods rule
	f.clock.Step(10 * time.Minute)
	fakePai.Spec.Policy.Upscale.Strategy = pointer.Ptr(datadoghq.DatadogPodAutoscalerDisabledStrategySelect)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 100,
		statusReplicas:  100,
		recReplicas:     120,
		scaleReplicas:   100,
		scaleError:      testutil.NewErrorString("upscaling disabled by strategy"),
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Verify downscale works as expected
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 100,
		statusReplicas:  100,
		recReplicas:     95,
		scaleReplicas:   95,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Setting Downscaling strategy to Disabled, nothing allowed
	// Moving clock 10 minutes forward to avoid the 5 pods rule
	f.clock.Step(10 * time.Minute)
	fakePai.Spec.Policy.Downscale.Strategy = pointer.Ptr(datadoghq.DatadogPodAutoscalerDisabledStrategySelect)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghq.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 95,
		statusReplicas:  95,
		recReplicas:     90,
		scaleReplicas:   95,
		scaleError:      testutil.NewErrorString("downscaling disabled by strategy"),
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)
}
