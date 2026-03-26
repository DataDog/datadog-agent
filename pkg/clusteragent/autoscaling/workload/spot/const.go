// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

// Spot scheduling constants.
const (
	// SpotEnabledLabelSelector is used to opt-in workload into spot scheduling.
	SpotEnabledLabelSelector = "autoscaling.datadoghq.com/spot-enabled=true"
	// SpotPercentageAnnotation is the annotation key for the desired percentage of replicas on spot (0-100)
	SpotPercentageAnnotation = "autoscaling.datadoghq.com/spot-percentage"
	// SpotMinOnDemandReplicasAnnotation is the annotation key for the minimum number of on-demand replicas
	SpotMinOnDemandReplicasAnnotation = "autoscaling.datadoghq.com/spot-min-on-demand-replicas"

	// SpotAssignedLabel is the label key set by the admission webhook on pods assigned to spot instances.
	SpotAssignedLabel = "autoscaling.datadoghq.com/spot-assigned"
	// SpotAssignedSpot is the SpotAssignedLabel value for pods assigned to spot instances.
	SpotAssignedSpot = "true"

	// KarpenterCapacityTypeLabel is the Karpenter node label for capacity type
	KarpenterCapacityTypeLabel = "karpenter.sh/capacity-type"
	// KarpenterCapacityTypeSpot is the Karpenter capacity type value for spot instances
	KarpenterCapacityTypeSpot = "spot"
)
