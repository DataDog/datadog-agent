// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

// Spot scheduling constants.
const (
	// SpotEnabledLabelKey is the label key used to opt-in workload into spot scheduling.
	SpotEnabledLabelKey = "autoscaling.datadoghq.com/spot-enabled"
	// SpotEnabledLabelValue is the label value used to opt-in workload into spot scheduling.
	SpotEnabledLabelValue = "true"

	// SpotConfigAnnotation is the annotation key for per-workload spot configuration
	// encoded as a JSON object with optional fields: percentage (int 0-100) and minOnDemandReplicas (int >= 0).
	// Example: {"percentage": 50, "minOnDemandReplicas": 1}
	SpotConfigAnnotation = "autoscaling.datadoghq.com/spot-config"

	// SpotDisabledUntilAnnotation is the annotation key for the timestamp until spot scheduling is disabled (RFC3339).
	SpotDisabledUntilAnnotation = "autoscaling.datadoghq.com/spot-disabled-until"

	// SpotAssignedLabel is the label key set by the admission webhook on pods assigned to spot instances.
	SpotAssignedLabel = "autoscaling.datadoghq.com/spot-assigned"
	// SpotAssignedLabelValue is the SpotAssignedLabel value for pods assigned to spot instances.
	SpotAssignedLabelValue = "true"
)
