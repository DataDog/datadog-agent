// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
)

// DatadogPodAutoscalerPausedCondition indicates that every action of the autoscaler is paused
// by the pause annotation. Declared here rather than in the CRD API package: the condition type
// carries no enum validation, so surfacing a new one requires no CRD change.
const DatadogPodAutoscalerPausedCondition datadoghqcommon.DatadogPodAutoscalerConditionType = "Paused"

const (
	// PreviewAnnotationKey is the annotation key used to enable preview/alpha autoscaling features.
	// Its value is a JSON object where each key enables a specific feature flag, e.g.:
	//   autoscaling.datadoghq.com/preview: '{"burstable":true}'
	// WARNING: preview features are experimental. Any option may be changed or
	// removed without notice in a future version.
	// Known keys:
	//   "burstable" (bool) — when true, CPU limits are removed from containers so they can burst
	//                        beyond their CPU request when spare capacity is available on the node.
	PreviewAnnotationKey = "autoscaling.datadoghq.com/preview"

	// PauseAnnotationKey is the annotation key used to pause every action of an autoscaler.
	// Value is a boolean, e.g.:
	//   autoscaling.datadoghq.com/pause: "true"
	// While paused, the autoscaler keeps computing and reporting recommendations but applies
	// nothing: no horizontal scaling, no vertical rollout or in-place resize, no POD patching
	// and no local fallback. Manual changes made to the target workload (replicas, container
	// resources) are left untouched.
	// A value that is not a valid boolean is ignored, as if the annotation were absent: a typo
	// must not be able to freeze autoscaling on a workload indefinitely.
	PauseAnnotationKey = "autoscaling.datadoghq.com/pause"

	// ForceFallbackAnnotationKey is the annotation key used to force the local recommender as
	// the active source of recommendations, even when product values are not stale.
	// Value is a boolean, e.g.:
	//   autoscaling.datadoghq.com/force-fallback: "true"
	// WARNING: "false" means "do not force", i.e. the default staleness-driven behaviour. It
	// does NOT disable the local fallback, which remains `spec.fallback.horizontal.enabled`.
	ForceFallbackAnnotationKey = "autoscaling.datadoghq.com/force-fallback"

	// RecommendationIDAnnotation is the annotation key used to store the recommendation ID
	RecommendationIDAnnotation = "autoscaling.datadoghq.com/rec-id"
	// AutoscalerIDAnnotation is the annotation key used to store the autoscaler ID
	AutoscalerIDAnnotation = "autoscaling.datadoghq.com/autoscaler-id"
	// RecommendationAppliedEventGeneratedAnnotation is an annotation added when even was generated for applied recommendation
	RecommendationAppliedEventGeneratedAnnotation = "autoscaling.datadoghq.com/event"
	// RolloutTimestampAnnotation is the annotation key used to store the rollout timestamp
	RolloutTimestampAnnotation = "autoscaling.datadoghq.com/rolloutAt"

	// RecommendationAppliedEventReason is the event reason when a recommendation is applied
	RecommendationAppliedEventReason = "RecommendationApplied"
	// SuccessfulScaleEventReason is the event reason when a scale operation is successful
	SuccessfulScaleEventReason = "SuccessfulScale"
	// FailedScaleEventReason is the event reason when a scale operation fails
	FailedScaleEventReason = "FailedScale"
	// SuccessfulTriggerRolloutEventReason is the event reason when a trigger rollout is successful
	SuccessfulTriggerRolloutEventReason = "SuccessfulTriggerRollout"
	// FailedTriggerRolloutEventReason is the event reason when a trigger rollout fails
	FailedTriggerRolloutEventReason = "FailedTriggerRollout"
	// ResizeSuccessfulEventReason is the event reason when all pods have completed an in-place resize cycle.
	ResizeSuccessfulEventReason = "ResizeSuccessful"
	// InPlaceEvictedEventReason is the event reason when a pod is evicted because it
	// could not be resized in-place (infeasible, deferred timeout, or resize error).
	InPlaceEvictedEventReason = "InPlaceEvicted"
	// FailedToEvictEventReason is the event reason when a pod could not be evicted
	FailedToEvictEventReason = "FailedToEvict"
)
