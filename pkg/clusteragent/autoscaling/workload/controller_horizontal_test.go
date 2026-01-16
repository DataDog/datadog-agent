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

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/common"
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

	// Pre-fetch scale subresource, mirroring what handleScaling does in the parent controller.
	var scale *autoscalingv1.Scale
	var gr schema.GroupResource
	var scaleErr error
	if autoscalerInternal.Spec() != nil {
		if gvk, err := autoscalerInternal.TargetGVK(); err == nil {
			scale, gr, scaleErr = f.scaler.get(context.Background(), fakePai.Namespace, autoscalerInternal.Spec().TargetRef.Name, gvk)
		}
	}

	res, err := f.controller.sync(context.Background(), fakeAutoscaler, &autoscalerInternal, scale, gr, scaleErr)
	return autoscalerInternal, res, err
}

type horizontalScalingTestArgs struct {
	fakePai          *model.FakePodAutoscalerInternal
	dataSource       datadoghqcommon.DatadogPodAutoscalerValueSource
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
	args.fakePai.HorizontalLastRecommendations = append(args.fakePai.HorizontalLastRecommendations, datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
		GeneratedAt: metav1.NewTime(f.clock.Now().Add(-args.dataOffset)),
		Replicas:    args.recReplicas,
	})

	autoscaler, result, err := f.runSync(args.fakePai)
	f.scaler.AssertNumberOfCalls(f.t, "get", 1)
	f.scaler.AssertNumberOfCalls(f.t, "update", expectedUpdateCalls)

	if scaleActionExpected && args.scaleError == nil {
		// Update fakePai with the new expected state
		action := &datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
			Time:                metav1.NewTime(f.clock.Now()),
			FromReplicas:        args.currentReplicas,
			ToReplicas:          args.scaleReplicas,
			RecommendedReplicas: pointer.Ptr(args.recReplicas),
		}
		if args.scaleLimitReason != "" {
			action.LimitedReason = &args.scaleLimitReason
		}

		args.fakePai.AddHorizontalAction(action.Time.Time, action)
		args.fakePai.HorizontalLastActionError = nil
		args.fakePai.HorizontalActionSuccessCount++
	} else if args.scaleError != nil {
		args.fakePai.HorizontalLastActionError = args.scaleError
		// Counter is only incremented when the scale update itself fails (not for internal errors like policy restrictions)
		if scaleActionExpected {
			args.fakePai.HorizontalActionErrorCount++
		}
	}
	// No scale action needed (fromReplicas == toReplicas): no counter increment

	args.fakePai.HorizontalLastLimitReason = args.scaleLimitReason

	model.AssertPodAutoscalersEqual(f.t, *args.fakePai, autoscaler)
	return result, err
}

func TestHorizontalControllerSyncPrerequisites(t *testing.T) {
	f := newHorizontalControllerFixture(t, time.Now())
	autoscalerNamespace := "default"
	autoscalerName := "test"

	fakePai := &model.FakePodAutoscalerInternal{
		Namespace:       autoscalerNamespace,
		Name:            autoscalerName,
		CurrentReplicas: pointer.Ptr[int32](5),
	}

	// Test case: no Spec, no action taken
	autoscaler, result, err := f.runSync(fakePai)
	assert.Equal(t, result, autoscaling.NoRequeue)
	assert.Nil(t, err)
	model.AssertPodAutoscalersEqual(t, fakePai.Build(), autoscaler)

	// Test case: Correct Spec and GVK, but no scaling values
	// Should do nothing
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
		ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
			ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
				StabilizationWindowSeconds: 0,
			},
			ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
				StabilizationWindowSeconds: 0,
			},
		},
	}
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
		Namespace:                  autoscalerNamespace,
		Name:                       autoscalerName,
		Spec:                       fakePai.Spec,
		CurrentReplicas:            pointer.Ptr[int32](5),
		TargetGVK:                  expectedGVK,
		HorizontalLastActionError:  testutil.NewErrorString("failed to get scale subresource for autoscaler default/test, err: some k8s error"),
		HorizontalActionErrorCount: 1,
	}, autoscaler)

	// Test case: Any scaling disabled by policy
	fakePai.Spec.ApplyPolicy = &datadoghq.DatadogPodAutoscalerApplyPolicy{
		Mode: datadoghq.DatadogPodAutoscalerApplyModePreview,
	}
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		currentReplicas: 5,
		statusReplicas:  5,
		recReplicas:     10,
		scaleReplicas:   5,
		scaleError:      testutil.NewErrorString("horizontal scaling disabled due to applyMode: Preview not allowing recommendations from source: Autoscaling"),
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test case: Fallback scaling direction disabled by policy
	fakePai.Spec.Fallback = &datadoghq.DatadogFallbackPolicy{
		Horizontal: datadoghq.DatadogPodAutoscalerHorizontalFallbackPolicy{
			Direction: datadoghq.DatadogPodAutoscalerFallbackDirectionScaleUp,
		},
	}
	fakePai.Spec.ApplyPolicy = &datadoghq.DatadogPodAutoscalerApplyPolicy{
		Mode: datadoghq.DatadogPodAutoscalerApplyModeApply,
	}
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerLocalValueSource,
		currentReplicas: 6,
		statusReplicas:  6,
		recReplicas:     5,
		scaleReplicas:   6,
		scaleError:      testutil.NewErrorString("scaling disabled as fallback in the scaling direction (scaleDown) is disabled"),
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test case: Fallback scaling direction unset
	fakePai.Spec.Fallback = &datadoghq.DatadogFallbackPolicy{
		Horizontal: datadoghq.DatadogPodAutoscalerHorizontalFallbackPolicy{},
	}
	fakePai.Spec.ApplyPolicy = &datadoghq.DatadogPodAutoscalerApplyPolicy{
		Mode: datadoghq.DatadogPodAutoscalerApplyModeApply,
	}
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerLocalValueSource,
		currentReplicas: 6,
		statusReplicas:  6,
		recReplicas:     7,
		scaleReplicas:   7,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Tests disabled until we add back Manual capability in the CRD
	// // Test case: Automatic scaling values disabled by policy
	// fakePai.Spec.ApplyPolicy = &datadoghq.DatadogPodAutoscalerApplyPolicy{
	// 	Mode: datadoghqcommon.DatadogPodAutoscalerManualApplyMode,
	// }
	// result, err = f.testScalingDecision(horizontalScalingTestArgs{
	// 	fakePai:         fakePai,
	// 	dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
	// 	currentReplicas: 5,
	// 	statusReplicas:  5,
	// 	recReplicas:     10,
	// 	scaleReplicas:   5,
	// 	scaleError:      testutil.NewErrorString("horizontal scaling disabled due to applyMode: Manual not allowing recommendations from source: Autoscaling"),
	// })
	// assert.Equal(t, autoscaling.NoRequeue, result)
	// assert.NoError(t, err)

	// // Test case: Automatic scaling values disabled by policy, sending manual value, should scale
	// fakePai.Spec.ApplyPolicy = &datadoghq.DatadogPodAutoscalerApplyPolicy{
	// 	Mode: datadoghq.DatadogPodAutoscalerManualApplyMode,
	// }
	// result, err = f.testScalingDecision(horizontalScalingTestArgs{
	// 	fakePai:         fakePai,
	// 	dataSource:      datadoghqcommon.DatadogPodAutoscalerManualValueSource,
	// 	currentReplicas: 5,
	// 	statusReplicas:  5,
	// 	recReplicas:     10,
	// 	scaleReplicas:   10,
	// })
	// assert.Equal(t, autoscaling.NoRequeue, result)
	// assert.NoError(t, err)
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
				Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp: f.clock.Now().Add(-defaultStepDuration),
				Replicas:  5,
			},
		},
		TargetGVK:       expectedGVK,
		CurrentReplicas: pointer.Ptr[int32](5),
	}

	// Step: same number of replicas, no action taken, only updating status
	result, err := f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
	fakePai.Spec.Constraints = &datadoghqcommon.DatadogPodAutoscalerConstraints{
		MinReplicas: pointer.Ptr[int32](2),
		MaxReplicas: pointer.Ptr[int32](8),
	}
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
	fakePai.Spec.Constraints = &datadoghqcommon.DatadogPodAutoscalerConstraints{
		MinReplicas: pointer.Ptr[int32](2),
		MaxReplicas: pointer.Ptr[int32](8),
	}
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
	fakePai.Spec.Constraints = &datadoghqcommon.DatadogPodAutoscalerConstraints{
		MinReplicas: pointer.Ptr[int32](8),
		MaxReplicas: pointer.Ptr[int32](10),
	}
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
	fakePai.Spec.Constraints = &datadoghqcommon.DatadogPodAutoscalerConstraints{
		MinReplicas: pointer.Ptr[int32](8),
		MaxReplicas: pointer.Ptr[int32](10),
	}
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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

func TestHorizontalControllerSyncScaleUpWithRules(t *testing.T) {
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
	// Rules used on scale up
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
			ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Strategy: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerMinChangeStrategySelect),
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          datadoghqcommon.DatadogPodAutoscalerPodsScalingRuleType,
							Value:         5,
							PeriodSeconds: 60,
						},
						{
							Type:          datadoghqcommon.DatadogPodAutoscalerPercentScalingRuleType,
							Value:         20,
							PeriodSeconds: 300,
						},
						{
							Type:          datadoghqcommon.DatadogPodAutoscalerPercentScalingRuleType,
							Value:         100,
							PeriodSeconds: 1800,
						},
					},
				},
			},
		},
		ScalingValues: model.ScalingValues{
			Horizontal: &model.HorizontalScalingValues{
				Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
	fakePai.HorizontalLastActions = append(fakePai.HorizontalLastActions, datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
		Time:                metav1.NewTime(f.clock.Now()),
		FromReplicas:        120,
		ToReplicas:          198,
		RecommendedReplicas: pointer.Ptr[int32](198),
	})

	// Moving clock 6 minutes forward to only block on 100% rule
	f.clock.Step(6 * time.Minute)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
	fakePai.Spec.ApplyPolicy.ScaleUp.Strategy = pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerMaxChangeStrategySelect)
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  200,
		statusReplicas:   200,
		recReplicas:      250,
		scaleReplicas:    238,
		scaleLimitReason: "desired replica count limited to 238 (originally 250) due to scaling policy",
	})
	assert.Equal(t, autoscaling.Requeue.After(31*time.Second), result)
	assert.NoError(t, err)
}

func TestHorizontalControllerSyncScaleDownWithRules(t *testing.T) {
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
	// Rules used on scale down
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
			ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Strategy: pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerMinChangeStrategySelect),
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          datadoghqcommon.DatadogPodAutoscalerPodsScalingRuleType,
							Value:         5,
							PeriodSeconds: 60,
						},
						{
							Type:          datadoghqcommon.DatadogPodAutoscalerPercentScalingRuleType,
							Value:         20,
							PeriodSeconds: 300,
						},
						{
							Type:          datadoghqcommon.DatadogPodAutoscalerPercentScalingRuleType,
							Value:         50,
							PeriodSeconds: 1800,
						},
					},
				},
			},
		},
		ScalingValues: model.ScalingValues{
			Horizontal: &model.HorizontalScalingValues{
				Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
	fakePai.HorizontalLastActions = append(fakePai.HorizontalLastActions, datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
		Time:                metav1.NewTime(f.clock.Now()),
		FromReplicas:        80,
		ToReplicas:          52,
		RecommendedReplicas: pointer.Ptr[int32](52),
	})

	// Moving clock 6 minutes forward to only block on 50% rule
	f.clock.Step(6 * time.Minute)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
	fakePai.Spec.ApplyPolicy.ScaleDown.Strategy = pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerMaxChangeStrategySelect)
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  50,
		statusReplicas:   50,
		recReplicas:      40,
		scaleReplicas:    41,
		scaleLimitReason: "desired replica count limited to 41 (originally 40) due to scaling policy",
	})
	assert.Equal(t, autoscaling.Requeue.After(31*time.Second), result)
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

	// Single rule on scale up and scale down
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
			Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
				MinReplicas: pointer.Ptr[int32](90),
				MaxReplicas: pointer.Ptr[int32](120),
			},
			ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          datadoghqcommon.DatadogPodAutoscalerPodsScalingRuleType,
							Value:         5,
							PeriodSeconds: 300,
						},
					},
				},
				ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					Rules: []datadoghqcommon.DatadogPodAutoscalerScalingRule{
						{
							Type:          datadoghqcommon.DatadogPodAutoscalerPodsScalingRuleType,
							Value:         5,
							PeriodSeconds: 300,
						},
					},
				},
			},
		},
		ScalingValues: model.ScalingValues{
			Horizontal: &model.HorizontalScalingValues{
				Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp: f.clock.Now().Add(-defaultStepDuration),
				Replicas:  100,
			},
		},
		TargetGVK:                 expectedGVK,
		HorizontalEventsRetention: 5 * time.Minute, // Matching rules
	}

	// Test scale up to 103 replicas (not limited)
	f.clock.Step(defaultStepDuration)
	result, err := f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 100,
		statusReplicas:  100,
		recReplicas:     103,
		scaleReplicas:   103,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test scale down to 97 replicas (not limited)
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 103,
		statusReplicas:  103,
		recReplicas:     97,
		scaleReplicas:   97,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test scale down to 92 replicas, limited to 95
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  97,
		statusReplicas:   97,
		recReplicas:      92,
		scaleReplicas:    95,
		scaleLimitReason: "desired replica count limited to 95 (originally 92) due to scaling policy",
	})
	assert.Equal(t, autoscaling.Requeue.After(241*time.Second), result)
	assert.NoError(t, err)

	// Test scale up to 110 replicas, limited to 105
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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
	fakePai.Spec.ApplyPolicy.ScaleUp.Strategy = pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerDisabledStrategySelect)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 100,
		statusReplicas:  100,
		recReplicas:     120,
		scaleReplicas:   100,
		scaleError:      testutil.NewErrorString("upscaling disabled by strategy"),
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Verify scale down works as expected
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 100,
		statusReplicas:  100,
		recReplicas:     95,
		scaleReplicas:   95,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Setting Downscaling strategy to Disabled, Upscaling allowed
	// Moving clock 10 minutes forward to avoid the 5 pods rule
	f.clock.Step(10 * time.Minute)
	fakePai.Spec.ApplyPolicy.ScaleDown.Strategy = pointer.Ptr(datadoghqcommon.DatadogPodAutoscalerDisabledStrategySelect)
	fakePai.Spec.ApplyPolicy.ScaleUp.Strategy = nil
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
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

func TestStabilizeRecommendations(t *testing.T) {
	currentTime := time.Now()

	tests := []struct {
		name                string
		lastRecommendations []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation
		currentReplicas     int32
		recReplicas         int32
		expected            int32
		expectedReason      string
		scaleUpWindow       int32
		scaleDownWindow     int32
		scaleDirection      common.ScaleDirection
	}{
		{
			name: "no scale down stabilization - constant scale up",
			lastRecommendations: []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
				{GeneratedAt: metav1.NewTime(currentTime.Add(-60 * time.Second)), Replicas: 6},
				{GeneratedAt: metav1.NewTime(currentTime.Add(-30 * time.Second)), Replicas: 4},
			},
			currentReplicas: 4,
			recReplicas:     8,
			expected:        8,
			expectedReason:  "",
			scaleUpWindow:   0,
			scaleDownWindow: 300,
			scaleDirection:  common.ScaleUp,
		},
		{
			name: "scale down stabilization",
			lastRecommendations: []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
				{GeneratedAt: metav1.NewTime(currentTime.Add(-60 * time.Second)), Replicas: 6},
				{GeneratedAt: metav1.NewTime(currentTime.Add(-30 * time.Second)), Replicas: 5},
			},
			currentReplicas: 5,
			recReplicas:     4,
			expected:        5,
			expectedReason:  "desired replica count adjusted to 5 (originally 4) due to stabilization window",
			scaleUpWindow:   0,
			scaleDownWindow: 300,
			scaleDirection:  common.ScaleDown,
		},
		{
			name: "scale down stabilization, recommendation flapping",
			lastRecommendations: []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
				{GeneratedAt: metav1.NewTime(currentTime.Add(-90 * time.Second)), Replicas: 6},
				{GeneratedAt: metav1.NewTime(currentTime.Add(-60 * time.Second)), Replicas: 9},
				{GeneratedAt: metav1.NewTime(currentTime.Add(-30 * time.Second)), Replicas: 7},
			},
			currentReplicas: 7,
			recReplicas:     5,
			expected:        7,
			expectedReason:  "desired replica count adjusted to 7 (originally 5) due to stabilization window",
			scaleUpWindow:   0,
			scaleDownWindow: 300,
			scaleDirection:  common.ScaleDown,
		},
		{
			name: "scale up stabilization",
			lastRecommendations: []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
				{GeneratedAt: metav1.NewTime(currentTime.Add(-60 * time.Second)), Replicas: 6},
				{GeneratedAt: metav1.NewTime(currentTime.Add(-30 * time.Second)), Replicas: 8},
			},
			currentReplicas: 8,
			recReplicas:     12,
			expected:        8,
			expectedReason:  "desired replica count adjusted to 8 (originally 12) due to stabilization window",
			scaleUpWindow:   300,
			scaleDownWindow: 0,
			scaleDirection:  common.ScaleUp,
		},
		{
			name: "scale up stabilization, recommendation flapping",
			lastRecommendations: []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
				{GeneratedAt: metav1.NewTime(currentTime.Add(-90 * time.Second)), Replicas: 8},
				{GeneratedAt: metav1.NewTime(currentTime.Add(-60 * time.Second)), Replicas: 7},
				{GeneratedAt: metav1.NewTime(currentTime.Add(-30 * time.Second)), Replicas: 9},
			},
			currentReplicas: 9,
			recReplicas:     12,
			expected:        9,
			expectedReason:  "desired replica count adjusted to 9 (originally 12) due to stabilization window",
			scaleUpWindow:   300,
			scaleDownWindow: 0,
			scaleDirection:  common.ScaleUp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recommendedReplicas, limitReason := stabilizeRecommendations(currentTime, tt.lastRecommendations, tt.currentReplicas, tt.recReplicas, tt.scaleUpWindow, tt.scaleDownWindow)
			assert.Equal(t, tt.expected, recommendedReplicas)
			assert.Equal(t, tt.expectedReason, limitReason)
		})
	}
}

func TestHorizontalControllerSyncScaleDownWithStabilization(t *testing.T) {
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
			Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
				MinReplicas: pointer.Ptr[int32](90),
				MaxReplicas: pointer.Ptr[int32](120),
			},
			ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					StabilizationWindowSeconds: 0,
				},
				ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					StabilizationWindowSeconds: 300,
				},
			},
		},
		HorizontalLastRecommendations: []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
			{GeneratedAt: metav1.NewTime(f.clock.Now().Add(-60 * time.Second)), Replicas: 94},
			{GeneratedAt: metav1.NewTime(f.clock.Now().Add(-30 * time.Second)), Replicas: 97},
		},
		TargetGVK:                          expectedGVK,
		HorizontalRecommendationsRetention: 5 * time.Minute,
	}

	// Test scale up to 100 replicas (not limited)
	f.clock.Step(defaultStepDuration)
	result, err := f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 97,
		statusReplicas:  97,
		recReplicas:     100,
		scaleReplicas:   100,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test scale down to 97 replicas, limited to 100
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  100,
		statusReplicas:   100,
		recReplicas:      97,
		scaleReplicas:    100,
		scaleLimitReason: "desired replica count adjusted to 100 (originally 97) due to stabilization window",
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test scale down to 95 replicas, still limited to 100
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  100,
		statusReplicas:   100,
		recReplicas:      95,
		scaleReplicas:    100,
		scaleLimitReason: "desired replica count adjusted to 100 (originally 95) due to stabilization window",
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test scale down to 92 replicas (not limited)
	// Moving clock 5 minutes forward to get past stabilization window
	f.clock.Step(5 * time.Minute)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 100,
		statusReplicas:  100,
		recReplicas:     92,
		scaleReplicas:   92,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test scale up to 100 replicas (not limited)
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 92,
		statusReplicas:  92,
		recReplicas:     100,
		scaleReplicas:   100,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)
}

func TestHorizontalControllerSyncScaleUpWithStabilization(t *testing.T) {
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
			Constraints: &datadoghqcommon.DatadogPodAutoscalerConstraints{
				MinReplicas: pointer.Ptr[int32](90),
				MaxReplicas: pointer.Ptr[int32](120),
			},
			ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					StabilizationWindowSeconds: 300,
				},
				ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					StabilizationWindowSeconds: 0,
				},
			},
		},
		HorizontalLastRecommendations: []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
			{GeneratedAt: metav1.NewTime(f.clock.Now().Add(-60 * time.Second)), Replicas: 110, Source: datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource},
			{GeneratedAt: metav1.NewTime(f.clock.Now().Add(-30 * time.Second)), Replicas: 104, Source: datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource},
		},
		ScalingValues: model.ScalingValues{
			Horizontal: &model.HorizontalScalingValues{
				Source:    datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
				Timestamp: f.clock.Now().Add(-defaultStepDuration),
				Replicas:  100,
			},
		},
		TargetGVK:                 expectedGVK,
		HorizontalEventsRetention: 5 * time.Minute,
	}

	// Test scale down to 100 replicas (not limited)
	f.clock.Step(defaultStepDuration)
	result, err := f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 104,
		statusReplicas:  104,
		recReplicas:     100,
		scaleReplicas:   100,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test scale up to 102 replicas, limited to 100
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  100,
		statusReplicas:   100,
		recReplicas:      102,
		scaleReplicas:    100,
		scaleLimitReason: "desired replica count adjusted to 100 (originally 102) due to stabilization window",
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test scale up to 105 replicas, still limited to 100
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  100,
		statusReplicas:   100,
		recReplicas:      105,
		scaleReplicas:    100,
		scaleLimitReason: "desired replica count adjusted to 100 (originally 105) due to stabilization window",
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test scale up to 102 replicas (not limited)
	// Moving clock 4 minutes forward to get past stabilization window
	f.clock.Step(4 * time.Minute)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 100,
		statusReplicas:  100,
		recReplicas:     102,
		scaleReplicas:   102,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Test scale down to 100 replicas (not limited)
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 102,
		statusReplicas:  102,
		recReplicas:     100,
		scaleReplicas:   100,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)
}

// See good explanation of stabilization envelope in https://github.com/kubernetes/kubernetes/issues/96671
func TestHorizontalControllerSyncScaleWithBothStabilizationWindows(t *testing.T) {
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

	// Both stabilization windows set to 5 minutes.
	// We seed recommendation history with values that create an envelope
	// around the current replica count so that we can verify that no scaling
	// happens while within the envelope and scaling resumes once bounds expire.
	fakePai := &model.FakePodAutoscalerInternal{
		Namespace: autoscalerNamespace,
		Name:      autoscalerName,
		Spec: &datadoghq.DatadogPodAutoscalerSpec{
			TargetRef: v2.CrossVersionObjectReference{
				Name:       autoscalerName,
				Kind:       expectedGVK.Kind,
				APIVersion: expectedGVK.Group + "/" + expectedGVK.Version,
			},
			ApplyPolicy: &datadoghq.DatadogPodAutoscalerApplyPolicy{
				ScaleUp: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					StabilizationWindowSeconds: 300,
				},
				ScaleDown: &datadoghqcommon.DatadogPodAutoscalerScalingPolicy{
					StabilizationWindowSeconds: 300,
				},
			},
		},
		HorizontalLastRecommendations: []datadoghqcommon.DatadogPodAutoscalerHorizontalRecommendation{
			{GeneratedAt: metav1.NewTime(f.clock.Now().Add(-4 * time.Minute)), Replicas: 95, Source: datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource},
			{GeneratedAt: metav1.NewTime(f.clock.Now().Add(-2 * time.Minute)), Replicas: 105, Source: datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource},
		},
		TargetGVK:                          expectedGVK,
		HorizontalRecommendationsRetention: 5 * time.Minute,
	}

	// Step 1: Desired jumps to 120 but current (100) is inside the envelope [95, 120],
	// so no scaling should occur, and stabilization should report a limit reason.
	f.clock.Step(defaultStepDuration)
	result, err := f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  100,
		statusReplicas:   100,
		recReplicas:      120,
		scaleReplicas:    100,
		scaleLimitReason: "desired replica count adjusted to 100 (originally 120) due to stabilization window",
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Step 2: After 5 minutes the lower bound (95) expires; the envelope allows scaling up.
	// We should scale up to 120 now that we are outside the envelope.
	f.clock.Step(5 * time.Minute)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 100,
		statusReplicas:  100,
		recReplicas:     120,
		scaleReplicas:   120,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Step 3: Desired drops to 80 but recent high recommendation (120) keeps the
	// upper envelope bound at 120; current 120 is on the boundary, so no scale down.
	f.clock.Step(defaultStepDuration)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:          fakePai,
		dataSource:       datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:       defaultStepDuration,
		currentReplicas:  120,
		statusReplicas:   120,
		recReplicas:      80,
		scaleReplicas:    120,
		scaleLimitReason: "desired replica count adjusted to 120 (originally 80) due to stabilization window",
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)

	// Step 4: After 5 minutes the upper bound (120) expires; now current 120 is
	// outside the envelope and we scale down to 80.
	f.clock.Step(5 * time.Minute)
	result, err = f.testScalingDecision(horizontalScalingTestArgs{
		fakePai:         fakePai,
		dataSource:      datadoghqcommon.DatadogPodAutoscalerAutoscalingValueSource,
		dataOffset:      defaultStepDuration,
		currentReplicas: 120,
		statusReplicas:  120,
		recReplicas:     80,
		scaleReplicas:   80,
	})
	assert.Equal(t, autoscaling.NoRequeue, result)
	assert.NoError(t, err)
}
