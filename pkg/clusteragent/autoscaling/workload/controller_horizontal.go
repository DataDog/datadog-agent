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

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	scaleclient "k8s.io/client-go/scale"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/clock"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

type scaleDirection int

const (
	noScale   scaleDirection = -1
	scaleUp   scaleDirection = 0
	scaleDown scaleDirection = 1

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
	gvk, err := autoscalerInternal.GetTargetGVK()
	if err != nil {
		return autoscaling.NoRequeue, err
	}

	// Get the current scale of the target resource
	scale, gr, err := hr.scaler.get(ctx, autoscalerInternal.Namespace, autoscalerInternal.Spec.TargetRef.Name, gvk)
	if err != nil {
		return autoscaling.Requeue, fmt.Errorf("failed to get scale subresource for autoscaler %s, err: %w", autoscalerInternal.ID(), err)
	}

	// Update current replicas
	autoscalerInternal.CurrentReplicas = pointer.Ptr(scale.Status.Replicas)

	return hr.performScaling(ctx, podAutoscaler, autoscalerInternal, gr, scale)
}

func (hr *horizontalController) performScaling(ctx context.Context, podAutoscaler *datadoghq.DatadogPodAutoscaler, autoscalerInternal *model.PodAutoscalerInternal, gr schema.GroupResource, scale *autoscalingv1.Scale) (autoscaling.ProcessResult, error) {
	// No Horizontal scaling, nothing to do
	if autoscalerInternal.ScalingValues.Horizontal == nil {
		autoscalerInternal.HorizontalLastActionError = nil
		return autoscaling.NoRequeue, nil
	}

	currentDesiredReplicas := scale.Spec.Replicas
	replicasFromRec := autoscalerInternal.ScalingValues.Horizontal.Replicas

	// Handling min/max replicas
	minReplicas := defaultMinReplicas
	if autoscalerInternal.Spec.Constraints != nil && autoscalerInternal.Spec.Constraints.MinReplicas != nil {
		minReplicas = *autoscalerInternal.Spec.Constraints.MinReplicas
	}

	maxReplicas := defaultMaxReplicas
	if autoscalerInternal.Spec.Constraints != nil && autoscalerInternal.Spec.Constraints.MaxReplicas >= minReplicas {
		maxReplicas = autoscalerInternal.Spec.Constraints.MaxReplicas
	}

	// Compute the desired number of replicas based on recommendations, rules and constraints
	horizontalAction, err := hr.computeScaleAction(autoscalerInternal, autoscalerInternal.ScalingValues.Horizontal.Source, currentDesiredReplicas, replicasFromRec, minReplicas, maxReplicas)
	if err != nil {
		autoscalerInternal.HorizontalLastActionError = err
		return autoscaling.NoRequeue, nil
	}
	// We are already scaled
	if horizontalAction == nil {
		autoscalerInternal.HorizontalLastActionError = nil
		return autoscaling.NoRequeue, nil
	}

	scale.Spec.Replicas = horizontalAction.ToReplicas
	_, err = hr.scaler.update(ctx, gr, scale)
	if err != nil {
		err = fmt.Errorf("failed to scale target: %s/%s to %d replicas, err: %w", scale.Namespace, scale.Name, horizontalAction.ToReplicas, err)
		hr.eventRecorder.Event(podAutoscaler, corev1.EventTypeWarning, model.FailedScaleEventReason, err.Error())
		autoscalerInternal.HorizontalLastActionError = err
		return autoscaling.Requeue, err
	}

	log.Debugf("Scaled target: %s/%s from %d replicas to %d replicas", scale.Namespace, scale.Name, horizontalAction.FromReplicas, horizontalAction.ToReplicas)
	autoscalerInternal.HorizontalLastAction = horizontalAction
	autoscalerInternal.HorizontalLastActionError = nil
	hr.eventRecorder.Eventf(podAutoscaler, corev1.EventTypeNormal, model.SuccessfulScaleEventReason, "Scaled target: %s/%s from %d replicas to %d replicas", scale.Namespace, scale.Name, horizontalAction.FromReplicas, horizontalAction.ToReplicas)
	return autoscaling.NoRequeue, nil
}

func (hr *horizontalController) computeScaleAction(
	autoscalerInternal *model.PodAutoscalerInternal,
	source datadoghq.DatadogPodAutoscalerValueSource,
	currentDesiredReplicas, targetDesiredReplicas int32,
	minReplicas, maxReplicas int32,
) (*datadoghq.DatadogPodAutoscalerHorizontalAction, error) {
	// Bound the targetDesiredReplicas to min/max replicas
	computedReplicas := targetDesiredReplicas
	if computedReplicas > maxReplicas {
		computedReplicas = maxReplicas
	} else if computedReplicas < minReplicas {
		computedReplicas = minReplicas
	}

	var scaleDirection scaleDirection
	if computedReplicas == currentDesiredReplicas {
		scaleDirection = noScale
	} else if computedReplicas > currentDesiredReplicas {
		scaleDirection = scaleUp
	} else {
		scaleDirection = scaleDown
	}

	// No scaling needed
	if scaleDirection == noScale {
		return nil, nil
	}

	// TODO: Implement scaling constraints, currently only checking if allowed
	allowed, reason := isScalingAllowed(autoscalerInternal, source, scaleDirection)
	if !allowed {
		return nil, errors.New(reason)
	}

	return &datadoghq.DatadogPodAutoscalerHorizontalAction{
		FromReplicas: currentDesiredReplicas,
		ToReplicas:   computedReplicas,
		Time:         metav1.NewTime(hr.clock.Now()),
	}, nil
}

func isScalingAllowed(autoscalerInternal *model.PodAutoscalerInternal, source datadoghq.DatadogPodAutoscalerValueSource, direction scaleDirection) (bool, string) {
	// If we don't have spec, we cannot take decisions, should not happen.
	if autoscalerInternal.Spec == nil {
		return false, "pod autoscaling hasn't been initialized yet"
	}

	// By default, policy is to allow all
	if autoscalerInternal.Spec.Policy == nil {
		return true, ""
	}

	// We do have policies, checking if they allow this source
	if !model.ApplyModeAllowSource(autoscalerInternal.Spec.Policy.ApplyMode, source) {
		return false, fmt.Sprintf("horizontal scaling disabled due to applyMode: %s not allowing recommendations from source: %s", autoscalerInternal.Spec.Policy.ApplyMode, source)
	}

	// Check if scaling direction is allowed
	if direction == scaleUp && autoscalerInternal.Spec.Policy.Upscale != nil && autoscalerInternal.Spec.Policy.Upscale.Strategy != nil {
		if *autoscalerInternal.Spec.Policy.Upscale.Strategy == datadoghq.DatadogPodAutoscalerDisabledStrategySelect {
			return false, "upscaling disabled by strategy"
		}
	}
	if direction == scaleDown && autoscalerInternal.Spec.Policy.Downscale != nil && autoscalerInternal.Spec.Policy.Downscale.Strategy != nil {
		if *autoscalerInternal.Spec.Policy.Downscale.Strategy == datadoghq.DatadogPodAutoscalerDisabledStrategySelect {
			return false, "downscaling disabled by strategy"
		}
	}

	// No specific policy defined, defaulting to allow
	return true, ""
}
