// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	scaleclient "k8s.io/client-go/scale"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	defaultMinReplicas int32 = 1
	defaultMaxReplicas int32 = math.MaxInt32
)

type horizontalController struct {
	clock         clock.Clock
	eventRecorder record.EventRecorder
	scaler        scaler
}

func newHorizontalReconciler(clock clock.Clock, eventRecorder record.EventRecorder, restMapper apimeta.RESTMapper, scaleGetter scaleclient.ScalesGetter) *horizontalController {
	return &horizontalController{
		clock:         clock,
		eventRecorder: eventRecorder,
		scaler:        newScaler(restMapper, scaleGetter),
	}
}

func (hr *horizontalController) sync(ctx context.Context, podAutoscaler *datadoghq.DatadogPodAutoscaler, autoscalerInternal *model.PodAutoscalerInternal) (autoscaling.ProcessResult, error) {
	// If we have no Spec, nothing to do
	if autoscalerInternal.Spec() == nil {
		return autoscaling.NoRequeue, nil
	}

	// Get the GVK of the target resource
	gvk, err := autoscalerInternal.TargetGVK()
	if err != nil {
		// Resolving GVK is considered a global error, not updating horizontal last error
		autoscalerInternal.SetError(err)
		return autoscaling.NoRequeue, err
	}

	// Get the current scale of the target resource
	scale, gr, err := hr.scaler.get(ctx, autoscalerInternal.Namespace(), autoscalerInternal.Spec().TargetRef.Name, gvk)
	if err != nil {
		err = fmt.Errorf("failed to get scale subresource for autoscaler %s, err: %w", autoscalerInternal.ID(), err)
		autoscalerInternal.UpdateFromHorizontalAction(nil, err)
		return autoscaling.Requeue, err
	}

	return hr.performScaling(ctx, podAutoscaler, autoscalerInternal, gr, scale)
}

func (hr *horizontalController) performScaling(ctx context.Context, podAutoscaler *datadoghq.DatadogPodAutoscaler, autoscalerInternal *model.PodAutoscalerInternal, gr schema.GroupResource, scale *autoscalingv1.Scale) (autoscaling.ProcessResult, error) {
	autoscalerSpec := autoscalerInternal.Spec()

	// No Horizontal scaling, nothing to do
	scalingValues := autoscalerInternal.ScalingValues()
	if scalingValues.Horizontal == nil {
		return autoscaling.NoRequeue, nil
	}

	currentDesiredReplicas := scale.Spec.Replicas
	replicasFromRec := scalingValues.Horizontal.Replicas

	// Handling min/max replicas
	specConstraints := autoscalerSpec.Constraints
	minReplicas := defaultMinReplicas
	if specConstraints != nil && specConstraints.MinReplicas != nil {
		minReplicas = *specConstraints.MinReplicas
	}

	maxReplicas := defaultMaxReplicas
	if specConstraints != nil && specConstraints.MaxReplicas >= minReplicas {
		maxReplicas = specConstraints.MaxReplicas
	}

	// Compute the desired number of replicas based on recommendations, rules and constraints
	horizontalAction, nextEvalAfter, err := hr.computeScaleAction(autoscalerInternal, scalingValues.Horizontal.Source, currentDesiredReplicas, replicasFromRec, minReplicas, maxReplicas)
	if err != nil {
		autoscalerInternal.UpdateFromHorizontalAction(nil, err)
		return autoscaling.NoRequeue, nil
	}
	// We are already scaled
	if horizontalAction == nil {
		autoscalerInternal.UpdateFromHorizontalAction(nil, nil)
		return autoscaling.NoRequeue, nil
	}
	// Target replicas has not changed due to scaling rules
	if horizontalAction.FromReplicas == horizontalAction.ToReplicas {
		autoscalerInternal.UpdateFromHorizontalAction(horizontalAction, nil)
		if nextEvalAfter > 0 {
			return autoscaling.Requeue.After(nextEvalAfter), nil
		}
		return autoscaling.NoRequeue, nil
	}

	scale.Spec.Replicas = horizontalAction.ToReplicas
	_, err = hr.scaler.update(ctx, gr, scale)
	if err != nil {
		err = fmt.Errorf("failed to scale target: %s/%s to %d replicas, err: %w", scale.Namespace, scale.Name, horizontalAction.ToReplicas, err)
		hr.eventRecorder.Event(podAutoscaler, corev1.EventTypeWarning, model.FailedScaleEventReason, err.Error())
		autoscalerInternal.UpdateFromHorizontalAction(nil, err)

		telemetryHorizontalScaleActions.Inc(scale.Namespace, scale.Name, podAutoscaler.Name, string(scalingValues.Horizontal.Source), "error", le.JoinLeaderValue)
		return autoscaling.Requeue, err
	}

	telemetryHorizontalScaleActions.Inc(scale.Namespace, scale.Name, podAutoscaler.Name, string(scalingValues.Horizontal.Source), "ok", le.JoinLeaderValue)
	setHorizontalScaleAppliedRecommendations(
		float64(horizontalAction.ToReplicas),
		scale.Namespace,
		scale.Name,
		podAutoscaler.Name,
		string(scalingValues.Horizontal.Source),
	)

	log.Debugf("Scaled target: %s/%s from %d replicas to %d replicas", scale.Namespace, scale.Name, horizontalAction.FromReplicas, horizontalAction.ToReplicas)
	autoscalerInternal.UpdateFromHorizontalAction(horizontalAction, nil)
	hr.eventRecorder.Eventf(podAutoscaler, corev1.EventTypeNormal, model.SuccessfulScaleEventReason, "Scaled target: %s/%s from %d replicas to %d replicas", scale.Namespace, scale.Name, horizontalAction.FromReplicas, horizontalAction.ToReplicas)
	if nextEvalAfter > 0 {
		return autoscaling.Requeue.After(nextEvalAfter), nil
	}
	return autoscaling.NoRequeue, nil
}

func (hr *horizontalController) computeScaleAction(
	autoscalerInternal *model.PodAutoscalerInternal,
	source datadoghqcommon.DatadogPodAutoscalerValueSource,
	currentDesiredReplicas, targetDesiredReplicas int32,
	minReplicas, maxReplicas int32,
) (*datadoghqcommon.DatadogPodAutoscalerHorizontalAction, time.Duration, error) {
	// Check if we scaling has been disabled explicitly
	if currentDesiredReplicas == 0 {
		return nil, 0, errors.New("scaling disabled as current replicas is set to 0")
	}

	// Saving original targetDesiredReplicas
	originalTargetDesiredReplicas := targetDesiredReplicas

	// Check if current replicas is outside of min/max constraints and scale back to min/max
	scalingTimestamp := hr.clock.Now()
	outsideBoundaries := false
	if currentDesiredReplicas < minReplicas {
		targetDesiredReplicas = minReplicas
		outsideBoundaries = true
	} else if currentDesiredReplicas > maxReplicas {
		targetDesiredReplicas = maxReplicas
		outsideBoundaries = true
	}

	// Checking scale direction
	scaleDirection := common.GetScaleDirection(currentDesiredReplicas, targetDesiredReplicas)

	// No scaling needed
	if scaleDirection == common.NoScale {
		return nil, 0, nil
	}

	// Checking if scaling constraints allow this scaling
	autoscalerSpec := autoscalerInternal.Spec()
	allowed, reason := isScalingAllowed(autoscalerSpec, source, scaleDirection)
	if !allowed {
		log.Debugf("Scaling not allowed for autoscaler id: %s, scale direction: %s, scale reason: %s", autoscalerInternal.ID(), scaleDirection, reason)
		return nil, 0, errors.New(reason)
	}

	// Going back inside requested boundaries in one shot.
	// TODO: Should we apply scaling rules in this case?
	if outsideBoundaries {
		log.Debugf("Current replica count for autoscaler id: %s is outside of min/max constraints, scaling back to closest boundary: %d replicas", autoscalerInternal.ID(), targetDesiredReplicas)
		return &datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
			FromReplicas:        currentDesiredReplicas,
			ToReplicas:          targetDesiredReplicas,
			RecommendedReplicas: &originalTargetDesiredReplicas,
			Time:                metav1.NewTime(scalingTimestamp),
			LimitedReason:       pointer.Ptr(fmt.Sprintf("current replica count is outside of min/max constraints, scaling back to closest boundary: %d replicas", targetDesiredReplicas)),
		}, 0, nil
	}

	var evalAfter time.Duration
	var limitReason string

	// Scaling is allowed, applying Min/Max replicas constraints from Spec
	if targetDesiredReplicas > maxReplicas {
		targetDesiredReplicas = maxReplicas
		limitReason = fmt.Sprintf("desired replica count limited to %d (originally %d) due to max replicas constraint", maxReplicas, originalTargetDesiredReplicas)
	} else if targetDesiredReplicas < minReplicas {
		targetDesiredReplicas = minReplicas
		limitReason = fmt.Sprintf("desired replica count limited to %d (originally %d) due to min replicas constraint", minReplicas, originalTargetDesiredReplicas)
	}

	// Applying scaling rules if any
	var rulesLimitReason string
	var rulesLimitedReplicas int32
	var rulesNextEvalAfter time.Duration
	if scaleDirection == common.ScaleUp && autoscalerSpec.ApplyPolicy != nil {
		rulesLimitedReplicas, rulesNextEvalAfter, rulesLimitReason = applyScaleUpPolicy(scalingTimestamp, autoscalerInternal.HorizontalLastActions(), autoscalerSpec.ApplyPolicy.ScaleUp, currentDesiredReplicas, targetDesiredReplicas)
	} else if scaleDirection == common.ScaleDown && autoscalerSpec.ApplyPolicy != nil {
		rulesLimitedReplicas, rulesNextEvalAfter, rulesLimitReason = applyScaleDownPolicy(scalingTimestamp, autoscalerInternal.HorizontalLastActions(), autoscalerSpec.ApplyPolicy.ScaleDown, currentDesiredReplicas, targetDesiredReplicas)
	}
	// If rules had any effect, use values from rules
	if rulesLimitReason != "" {
		limitReason = rulesLimitReason
		targetDesiredReplicas = rulesLimitedReplicas
		// To make sure event has expired and not have sub-second requeue, will be rounded to the next second
		evalAfter = rulesNextEvalAfter.Truncate(time.Second) + time.Second
	}

	// Stabilize recommendation
	var stabilizationLimitReason string
	var stabilizationLimitedReplicas int32
	scaleUpStabilizationSeconds := int32(0)
	scaleDownStabilizationSeconds := int32(0)

	if policy := autoscalerInternal.Spec().ApplyPolicy; policy != nil {
		if scaleUpPolicy := policy.ScaleUp; scaleUpPolicy != nil {
			scaleUpStabilizationSeconds = int32(scaleUpPolicy.StabilizationWindowSeconds)
		}
		if scaleDownPolicy := policy.ScaleDown; scaleDownPolicy != nil {
			scaleDownStabilizationSeconds = int32(scaleDownPolicy.StabilizationWindowSeconds)
		}
	}

	stabilizationLimitedReplicas, stabilizationLimitReason = stabilizeRecommendations(scalingTimestamp, autoscalerInternal.HorizontalLastActions(), currentDesiredReplicas, targetDesiredReplicas, scaleUpStabilizationSeconds, scaleDownStabilizationSeconds, scaleDirection)
	if stabilizationLimitReason != "" {
		limitReason = stabilizationLimitReason
		targetDesiredReplicas = stabilizationLimitedReplicas
	}

	horizontalAction := &datadoghqcommon.DatadogPodAutoscalerHorizontalAction{
		FromReplicas:        currentDesiredReplicas,
		ToReplicas:          targetDesiredReplicas,
		RecommendedReplicas: &originalTargetDesiredReplicas,
		Time:                metav1.NewTime(scalingTimestamp),
	}
	if limitReason != "" {
		log.Debugf("Scaling limited for autoscaler id: %s, scale direction: %s, limit reason: %s", autoscalerInternal.ID(), scaleDirection, limitReason)
		horizontalAction.LimitedReason = pointer.Ptr(limitReason)
	}
	return horizontalAction, evalAfter, nil
}

func isScalingAllowed(autoscalerSpec *datadoghq.DatadogPodAutoscalerSpec, source datadoghqcommon.DatadogPodAutoscalerValueSource, direction common.ScaleDirection) (bool, string) {
	// If we don't have spec, we cannot take decisions, should not happen.
	if autoscalerSpec == nil {
		return false, "pod autoscaling hasn't been initialized yet"
	}

	// By default, policy is to allow all
	if autoscalerSpec.ApplyPolicy == nil {
		return true, ""
	}

	// Default apply mode to All if not set
	applyMode := autoscalerSpec.ApplyPolicy.Mode
	if applyMode == "" {
		applyMode = datadoghq.DatadogPodAutoscalerApplyModeApply
	}

	// We do have policies, checking if they allow this source
	if !model.ApplyModeAllowSource(applyMode, source) {
		return false, fmt.Sprintf("horizontal scaling disabled due to applyMode: %s not allowing recommendations from source: %s", autoscalerSpec.ApplyPolicy.Mode, source)
	}

	// Check if scaling direction is allowed
	if direction == common.ScaleUp && autoscalerSpec.ApplyPolicy.ScaleUp != nil && autoscalerSpec.ApplyPolicy.ScaleUp.Strategy != nil {
		if *autoscalerSpec.ApplyPolicy.ScaleUp.Strategy == datadoghqcommon.DatadogPodAutoscalerDisabledStrategySelect {
			return false, "upscaling disabled by strategy"
		}
	}
	if direction == common.ScaleDown && autoscalerSpec.ApplyPolicy.ScaleDown != nil && autoscalerSpec.ApplyPolicy.ScaleDown.Strategy != nil {
		if *autoscalerSpec.ApplyPolicy.ScaleDown.Strategy == datadoghqcommon.DatadogPodAutoscalerDisabledStrategySelect {
			return false, "downscaling disabled by strategy"
		}
	}

	// No specific policy defined, defaulting to allow
	return true, ""
}

func applyScaleUpPolicy(
	currentTime time.Time,
	events []datadoghqcommon.DatadogPodAutoscalerHorizontalAction,
	policy *datadoghqcommon.DatadogPodAutoscalerScalingPolicy,
	currentDesiredReplicas, targetDesiredReplicas int32,
) (int32, time.Duration, string) {
	if policy == nil {
		return targetDesiredReplicas, 0, ""
	}

	strategy := datadoghqcommon.DatadogPodAutoscalerMaxChangeStrategySelect
	// If no strategy is defined, we default to the max policy for scale up
	if policy.Strategy != nil {
		strategy = *policy.Strategy
	}

	var maxReplicasFromRules int32
	var selectStrategyFunc func(int32, int32) int32
	minExpireIn := time.Hour // We don't support more than 1 hour of events
	if strategy == datadoghqcommon.DatadogPodAutoscalerMinChangeStrategySelect {
		maxReplicasFromRules = math.MaxInt32
		selectStrategyFunc = min
	} else {
		maxReplicasFromRules = math.MinInt32
		selectStrategyFunc = max
	}

	for _, rule := range policy.Rules {
		// We could find directly `periodStartReplicas` by looking at `FromReplicas` in the first matching event.
		// TODO: In case of manual scaling (outside of DPA), we could consider it in the calculation, while it's currently not.
		replicasAdded, replicasRemoved, expireIn := accumulateReplicasChange(currentTime, events, rule.PeriodSeconds)
		minExpireIn = min(minExpireIn, expireIn)

		// When are computing the number of replicas at the start of the period, needed to compute % scaling.
		// For that we consider the current number and apply the opposite of the events that happened in the period.
		periodStartReplicas := currentDesiredReplicas - replicasAdded + replicasRemoved
		var ruleMax int32
		if rule.Type == datadoghqcommon.DatadogPodAutoscalerPodsScalingRuleType {
			ruleMax = periodStartReplicas + rule.Value
		} else if rule.Type == datadoghqcommon.DatadogPodAutoscalerPercentScalingRuleType {
			// 1.x * start may yield the same number of replicas as periodStartReplicas, ceiling up to always always allow at least 1 replica
			// otherwise it would block scaling up forever.
			ruleMax = int32(math.Ceil(float64(periodStartReplicas) * (1 + float64(rule.Value)/100)))
		}
		maxReplicasFromRules = selectStrategyFunc(maxReplicasFromRules, ruleMax)
	}

	// No rules matched, not restricting the scaling
	if maxReplicasFromRules == math.MaxInt32 || maxReplicasFromRules == math.MinInt32 {
		return targetDesiredReplicas, 0, ""
	}

	// If we already above what we are allowed to scale to, we should not scale further
	if currentDesiredReplicas > maxReplicasFromRules {
		maxReplicasFromRules = currentDesiredReplicas
	}

	// If we're limited by rules,
	if targetDesiredReplicas > maxReplicasFromRules {
		return maxReplicasFromRules, minExpireIn, fmt.Sprintf("desired replica count limited to %d (originally %d) due to scaling policy", maxReplicasFromRules, targetDesiredReplicas)
	}
	return targetDesiredReplicas, 0, ""
}

func applyScaleDownPolicy(
	currentTime time.Time,
	events []datadoghqcommon.DatadogPodAutoscalerHorizontalAction,
	policy *datadoghqcommon.DatadogPodAutoscalerScalingPolicy,
	currentDesiredReplicas, targetDesiredReplicas int32,
) (int32, time.Duration, string) {
	if policy == nil {
		return targetDesiredReplicas, 0, ""
	}

	strategy := datadoghqcommon.DatadogPodAutoscalerMaxChangeStrategySelect
	// If no strategy is defined, we default to the max policy for scale up
	if policy.Strategy != nil {
		strategy = *policy.Strategy
	}

	var minReplicasFromRules int32
	minExpireIn := time.Hour // We don't support more than 1 hour of events
	var selectPolicyFn func(int32, int32) int32
	if strategy == datadoghqcommon.DatadogPodAutoscalerMinChangeStrategySelect {
		minReplicasFromRules = math.MinInt32
		selectPolicyFn = max // For scaling down, the lowest change ('min' policy) produces a maximum value
	} else {
		minReplicasFromRules = math.MaxInt32
		selectPolicyFn = min
	}

	for _, rule := range policy.Rules {
		// We could find directly `periodStartReplicas` by looking at `FromReplicas` in the first matching event.
		// TODO: In case of manual scaling (outside of DPA), we could consider it in the calculation, while it's currently not.
		replicasAdded, replicasRemoved, expireIn := accumulateReplicasChange(currentTime, events, rule.PeriodSeconds)
		minExpireIn = min(minExpireIn, expireIn)

		// When are computing the number of replicas at the start of the period, needed to compute % scaling.
		// For that we consider the current number and apply the opposite of the events that happened in the period.
		periodStartReplicas := currentDesiredReplicas - replicasAdded + replicasRemoved
		var ruleMin int32
		if rule.Type == datadoghqcommon.DatadogPodAutoscalerPodsScalingRuleType {
			ruleMin = periodStartReplicas - rule.Value
		} else if rule.Type == datadoghqcommon.DatadogPodAutoscalerPercentScalingRuleType {
			// When casting, the decimal is truncated, so we always have at least 1 replica allowed
			ruleMin = int32(float64(periodStartReplicas) * (1 - float64(rule.Value)/100))
		}
		minReplicasFromRules = selectPolicyFn(minReplicasFromRules, ruleMin)
	}

	// No rules matched, not restricting the scaling
	if minReplicasFromRules == math.MaxInt32 || minReplicasFromRules == math.MinInt32 {
		return targetDesiredReplicas, 0, ""
	}

	// If we already below what we are allowed to scale to, we should not scale further
	if currentDesiredReplicas < minReplicasFromRules {
		minReplicasFromRules = currentDesiredReplicas
	}

	if targetDesiredReplicas < minReplicasFromRules {
		return minReplicasFromRules, minExpireIn, fmt.Sprintf("desired replica count limited to %d (originally %d) due to scaling policy", minReplicasFromRules, targetDesiredReplicas)
	}
	return targetDesiredReplicas, 0, ""
}

func accumulateReplicasChange(currentTime time.Time, events []datadoghqcommon.DatadogPodAutoscalerHorizontalAction, periodSeconds int32) (added, removed int32, expireIn time.Duration) {
	periodDuration := time.Duration(periodSeconds) * time.Second
	earliestTimestamp := currentTime.Add(-periodDuration)

	for _, event := range events {
		if event.Time.Time.After(earliestTimestamp) {
			if expireIn == 0 {
				// Record when the oldest event will be out of the window
				expireIn = event.Time.Sub(earliestTimestamp)
			}

			diff := event.ToReplicas - event.FromReplicas
			if diff > 0 {
				added += diff
			} else {
				removed += -diff
			}
		}
	}

	if expireIn <= 0 {
		expireIn = periodDuration
	}
	return
}

func stabilizeRecommendations(currentTime time.Time, pastActions []datadoghqcommon.DatadogPodAutoscalerHorizontalAction, currentReplicas int32, originalTargetDesiredReplicas int32, stabilizationWindowScaleUpSeconds int32, stabilizationWindowScaleDownSeconds int32, scaleDirection common.ScaleDirection) (int32, string) {
	limitReason := ""

	if len(pastActions) == 0 {
		return originalTargetDesiredReplicas, limitReason
	}

	upRecommendation := originalTargetDesiredReplicas
	upCutoff := currentTime.Add(-time.Duration(stabilizationWindowScaleUpSeconds) * time.Second)

	downRecommendation := originalTargetDesiredReplicas
	downCutoff := currentTime.Add(-time.Duration(stabilizationWindowScaleDownSeconds) * time.Second)

	for _, a := range pastActions {
		if scaleDirection == common.ScaleUp && a.Time.Time.After(upCutoff) {
			upRecommendation = min(upRecommendation, *a.RecommendedReplicas)
		}

		if scaleDirection == common.ScaleDown && a.Time.Time.After(downCutoff) {
			downRecommendation = max(downRecommendation, *a.RecommendedReplicas)
		}

		if (scaleDirection == common.ScaleUp && a.Time.Time.Before(upCutoff)) || (scaleDirection == common.ScaleDown && a.Time.Time.Before(downCutoff)) {
			break
		}
	}

	recommendation := currentReplicas
	if recommendation < upRecommendation {
		recommendation = upRecommendation
	}
	if recommendation > downRecommendation {
		recommendation = downRecommendation
	}
	if recommendation != originalTargetDesiredReplicas {
		limitReason = fmt.Sprintf("desired replica count limited to %d (originally %d) due to stabilization window", recommendation, originalTargetDesiredReplicas)
	}

	return recommendation, limitReason
}
