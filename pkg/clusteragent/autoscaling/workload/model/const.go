// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

// LocallyPausedReason is the Active condition reason while the pause annotation is set.
const LocallyPausedReason = "LocallyPaused"

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

	// PauseAnnotationKey ("true") stops all actions of the autoscaler: the workload stays as it is.
	PauseAnnotationKey = "autoscaling.datadoghq.com/pause"

	// ForceFallbackAnnotationKey ("true") triggers the local fallback as if recommendations were stale.
	ForceFallbackAnnotationKey = "autoscaling.datadoghq.com/force-fallback"

	// ForceReplicasAnnotationKey (e.g. "28") pins the replica count, bypassing constraints and rate rules.
	ForceReplicasAnnotationKey = "autoscaling.datadoghq.com/force-replicas"

	// RecommendationIDAnnotation is the annotation key used to store the recommendation ID
	RecommendationIDAnnotation = "autoscaling.datadoghq.com/rec-id"
	// RuntimeRecommendationIDAnnotation is the annotation key used to store a hash of the runtime values
	RuntimeRecommendationIDAnnotation = "autoscaling.datadoghq.com/runtime-rec-id"
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
