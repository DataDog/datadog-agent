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
	// SpotPercentageAnnotation is the annotation key for the desired percentage of replicas on spot (0-100)
	SpotPercentageAnnotation = "autoscaling.datadoghq.com/spot-percentage"
	// SpotMinOnDemandReplicasAnnotation is the annotation key for the minimum number of on-demand replicas
	SpotMinOnDemandReplicasAnnotation = "autoscaling.datadoghq.com/spot-min-on-demand-replicas"
	// SpotDisabledUntilAnnotation is the annotation key for the timestamp until spot scheduling is disabled (RFC3339).
	SpotDisabledUntilAnnotation = "autoscaling.datadoghq.com/spot-disabled-until"

	// SpotAssignedLabel is the label key set by the admission webhook on pods assigned to spot instances.
	SpotAssignedLabel = "autoscaling.datadoghq.com/spot-assigned"
	// SpotAssignedSpot is the SpotAssignedLabel value for pods assigned to spot instances.
	SpotAssignedSpot = "true"
)
