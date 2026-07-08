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

// Spot node label and taint.
// Use our own namespace so we control it independently of the cluster autoscaler.
const (
	spotNodeLabelKey   = "autoscaling.datadoghq.com/capacity-type"
	spotNodeLabelValue = "interruptible"
	spotNodeTaintKey   = "autoscaling.datadoghq.com/capacity-type"
	spotNodeTaintValue = "interruptible"
)

// Spot scheduling Kubernetes event reasons.
const (
	// EventReasonSpotRebalancingEviction is the event reason for a pod evicted by the rebalancer.
	EventReasonSpotRebalancingEviction = "SpotRebalancingEviction"
	// EventReasonSpotFallbackEviction is the event reason for a pending spot pod evicted during on-demand fallback.
	EventReasonSpotFallbackEviction = "SpotFallbackEviction"
	// EventReasonSpotSchedulingDisabled is the event reason when spot scheduling is disabled for a workload.
	EventReasonSpotSchedulingDisabled = "SpotSchedulingDisabled"
)

// Spot scheduling metrics.
const (
	metricPrefix = "datadog.cluster_agent.autoscaling.cluster.spot."

	// MetricNamePods is a gauge reporting the number of running pods by {kube_namespace, kube_<kind>, capacity_type}.
	MetricNamePods = metricPrefix + "pods"
	// MetricNameExcessPods is a gauge reporting the number of pods exceeding the target spot ratio by {kube_namespace, kube_<kind>, capacity_type}.
	MetricNameExcessPods = metricPrefix + "excess_pods"
	// MetricNameFallbacks is a counter incremented each time spot scheduling falls back to on-demand by {kube_namespace, kube_<kind>}.
	MetricNameFallbacks = metricPrefix + "fallbacks"
	// MetricNameRebalanceEvictions is a counter incremented each time the rebalancer evicts a pod by {kube_namespace, kube_<kind>, capacity_type}.
	MetricNameRebalanceEvictions = metricPrefix + "rebalance_evictions"
	// MetricNamePendingSeconds is a distribution of the time a spot pod spent in the Pending phase.
	MetricNamePendingSeconds = metricPrefix + "pending_seconds"

	// MetricNameWorkloads is a gauge reporting the total number of spot-enabled workloads by {workload_kind}.
	MetricNameWorkloads = metricPrefix + "workloads"
	// MetricNameActiveFallbacks is a gauge reporting the number of workloads currently in fallback mode by {workload_kind}.
	MetricNameActiveFallbacks = metricPrefix + "active_fallbacks"
)
