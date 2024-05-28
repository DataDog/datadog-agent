// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	"context"
	"fmt"
	"math"

	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	scaleclient "k8s.io/client-go/scale"

	datadoghq "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

type scaleDirection int

const (
	scaleUp   scaleDirection = 0
	scaleDown scaleDirection = 1

	defaultMinReplicas int32 = 1
	defaultMaxReplicas int32 = math.MaxInt32
)

type horizontalController struct {
	scaler scaler
}

func newHorizontalReconciler(restMapper apimeta.RESTMapper, scaleGetter scaleclient.ScalesGetter) *horizontalController {
	return &horizontalController{
		scaler: newScaler(restMapper, scaleGetter),
	}
}

func (hr *horizontalController) sync(ctx context.Context, autoscalerInternal *model.PodAutoscalerInternal) (autoscaling.ProcessResult, error) {
	gvk, err := autoscalerInternal.GetTargetGVK()
	if err != nil {
		return autoscaling.NoRequeue, err
	}

	// Get the current scale of the target resource
	scale, gr, err := hr.scaler.get(ctx, autoscalerInternal.Namespace, autoscalerInternal.Spec.TargetRef.Name, gvk)
	if err != nil {
		return autoscaling.Requeue, fmt.Errorf("failed to get scale subresource for autoscaler %s, err: %w", autoscalerInternal.ID(), err)
	}

	// Update the current number of replicas from the scaling values
	if autoscalerInternal.CurrentReplicas == nil || *autoscalerInternal.CurrentReplicas != scale.Spec.Replicas {
		autoscalerInternal.CurrentReplicas = pointer.Ptr(scale.Spec.Replicas)
	}

	action, err := hr.performScaling(ctx, autoscalerInternal, gr, scale)
	if err != nil {
		autoscalerInternal.HorizontalLastActionError = err
		return autoscaling.Requeue, err
	}
	if action != nil {
		autoscalerInternal.HorizontalLastAction = action
	}

	return autoscaling.NoRequeue, nil
}

func (hr *horizontalController) performScaling(ctx context.Context, autoscalerInternal *model.PodAutoscalerInternal, gr schema.GroupResource, scale *autoscalingv1.Scale) (*datadoghq.DatadogPodAutoscalerHorizontalAction, error) {
	// No update required, except perhaps status, bailing out
	if autoscalerInternal.ScalingValues.Horizontal == nil ||
		autoscalerInternal.ScalingValues.Horizontal.Replicas == scale.Spec.Replicas {
		return nil, nil
	}

	currentDesiredReplicas := scale.Spec.Replicas
	targetDesiredReplicas := autoscalerInternal.ScalingValues.Horizontal.Replicas

	// Handling min/max replicas
	minReplicas := defaultMinReplicas
	if autoscalerInternal.Spec.Constraints != nil && autoscalerInternal.Spec.Constraints.MinReplicas != nil {
		minReplicas = *autoscalerInternal.Spec.Constraints.MinReplicas
	}

	maxReplicas := defaultMaxReplicas
	if autoscalerInternal.Spec.Constraints != nil && autoscalerInternal.Spec.Constraints.MaxReplicas >= minReplicas {
		maxReplicas = autoscalerInternal.Spec.Constraints.MaxReplicas
	}

	var scaleDirection scaleDirection
	if targetDesiredReplicas > currentDesiredReplicas {
		scaleDirection = scaleUp
	} else {
		scaleDirection = scaleDown
	}

	scale.Spec.Replicas = hr.computeScaleAction(currentDesiredReplicas, targetDesiredReplicas, minReplicas, maxReplicas, scaleDirection)
	scaleResult, err := hr.scaler.update(ctx, gr, scale)
	if err != nil {
		return nil, fmt.Errorf("failed to update scale subresource for autoscaler %s, err: %w", autoscalerInternal.ID(), err)
	}

	// Use slightly newer data for the status update
	autoscalerInternal.CurrentReplicas = pointer.Ptr(scaleResult.Spec.Replicas)

	return &datadoghq.DatadogPodAutoscalerHorizontalAction{
		FromReplicas: currentDesiredReplicas,
		ToReplicas:   targetDesiredReplicas,
		Time:         metav1.Now(),
	}, nil
}

func (hr *horizontalController) computeScaleAction(
	_, targetDesiredReplicas int32,
	minReplicas, maxReplicas int32,
	_ scaleDirection,
) int32 {
	// TODO: Implement the logic to compute the new number of replicas based on Policies
	if targetDesiredReplicas > maxReplicas {
		targetDesiredReplicas = maxReplicas
	} else if targetDesiredReplicas < minReplicas {
		targetDesiredReplicas = minReplicas
	}

	return targetDesiredReplicas
}
