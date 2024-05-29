// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

const (
	// RecommendationIDAnnotation is the annotation key used to store the recommendation ID
	RecommendationIDAnnotation = "autoscaling.datadoghq.com/rec-id"

	// RecommendationAppliedEventReason is the event reason when a recommendation is applied
	RecommendationAppliedEventReason = "RecommendationApplied"
	// SuccessfulScaleEventReason is the event reason when a scale operation is successful
	SuccessfulScaleEventReason = "SuccessfulScale"
	// FailedScaleEventReason is the event reason when a scale operation fails
	FailedScaleEventReason = "FailedScale"
)
