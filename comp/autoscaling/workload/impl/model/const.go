// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package model

const (
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
)
