// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

import (
	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
)

const (
	// DatadogPodAutoscalerHPAMigrationCondition is true when the DPA has taken over management of an HPA.
	// This condition type is agent-local and not yet defined in the upstream operator API.
	DatadogPodAutoscalerHPAMigrationCondition datadoghqcommon.DatadogPodAutoscalerConditionType = "HPAMigration"

	// PreviewAnnotationKey is the annotation key used to enable preview/alpha autoscaling features.
	// Its value is a JSON object where each key enables a specific feature flag, e.g.:
	//   autoscaling.datadoghq.com/preview: '{"burstable":true}'
	// WARNING: preview features are experimental. Any option may be changed or
	// removed without notice in a future version.
	// Known keys:
	//   "burstable"     (bool) — when true, CPU limits are removed from containers so they can burst
	//                            beyond their CPU request when spare capacity is available on the node.
	//   "hpa-migration" (bool) — when true, enables HPA-to-DPA migration: the controller will detect
	//                            an existing HPA for the same target, neutralise it, and take over
	//                            horizontal scaling.
	PreviewAnnotationKey = "autoscaling.datadoghq.com/preview"

	// HPAOriginalSpecAnnotation is the annotation key placed on an HPA resource to store its original
	// spec as a JSON string. Used to restore the HPA when the managing DPA is deleted.
	HPAOriginalSpecAnnotation = "autoscaling.datadoghq.com/hpa-original-spec"

	// HPAManagedByDPAAnnotation is the annotation key placed on an HPA resource to record which
	// DatadogPodAutoscaler is currently managing it. Value is "<namespace>/<name>".
	HPAManagedByDPAAnnotation = "autoscaling.datadoghq.com/managed-by-dpa"

	// HPAConfigImportedAnnotation is the annotation key placed on a DPA to mark that the HPA
	// configuration has already been one-shot imported into the DPA spec (UC2). Prevents re-importing
	// on subsequent reconciliations and overwriting user edits.
	HPAConfigImportedAnnotation = "autoscaling.datadoghq.com/hpa-config-imported"

	// HPAMigrationFinalizer is the finalizer added to a DPA when it takes over management of an HPA.
	// It ensures the HPA is restored to its original state before the DPA object is fully removed.
	HPAMigrationFinalizer = "autoscaling.datadoghq.com/hpa-migration"

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
