// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package workload

import (
	corev1 "k8s.io/api/core/v1"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	workqueuetelemetry "github.com/DataDog/datadog-agent/pkg/util/workqueue/telemetry"
)

const (
	subsystem = "autoscaling_workload"

	allContainerLabelContainerNameValue = "all_containers"
)

var (
	autoscalingQueueMetricsProvider = workqueuetelemetry.NewQueueMetricsProvider()
	commonOpts                      = telemetry.Options{NoDoubleUnderscoreSep: true}

	// Common label definitions for DPA metrics
	dpaTelemetryLabels                      = []string{"namespace", "target_name", "autoscaler_name", le.JoinLeaderLabel}
	dpaSpecContainerTelemetryLabels         = []string{"namespace", "target_name", "autoscaler_name", "target_container_name", le.JoinLeaderLabel}
	dpaStatusContainerTelemetryLabels       = []string{"namespace", "target_name", "autoscaler_name", "source", "target_container_name", le.JoinLeaderLabel}
	dpaStatusContainerTelemetryLegacyLabels = []string{"namespace", "target_name", "autoscaler_name", "source", "container_name", "target_container_name", "resource_name", le.JoinLeaderLabel}

	// telemetryReceivedRecommendationsVersion tracks the version of the received recommendations by the config retriever
	telemetryReceivedRecommendationsVersion = telemetry.NewGaugeWithOpts(
		subsystem,
		"received_recommendations_version",
		dpaTelemetryLabels,
		"Tracks the version of the received recommendations by the config retriever",
		commonOpts,
	)

	// telemetryHorizontalScaleActions tracks the number of horizontal scaling attempts
	telemetryHorizontalScaleActions = telemetry.NewCounterWithOpts(
		subsystem,
		"horizontal_scaling_actions",
		[]string{"namespace", "target_name", "autoscaler_name", "source", "status", le.JoinLeaderLabel},
		"Tracks the number of horizontal scale events done",
		commonOpts,
	)
	// telemetryHorizontalScaleReceivedRecommendations tracks the horizontal scaling recommendation values received
	telemetryHorizontalScaleReceivedRecommendations = telemetry.NewGaugeWithOpts(
		subsystem,
		"horizontal_scaling_received_replicas",
		[]string{"namespace", "target_name", "autoscaler_name", "source", le.JoinLeaderLabel},
		"Tracks the value of replicas applied by the horizontal scaling recommendation",
		commonOpts,
	)
	// telemetryHorizontalScaleAppliedRecommendations tracks the horizontal scaling recommendation values applied
	telemetryHorizontalScaleAppliedRecommendations = telemetry.NewGaugeWithOpts(
		subsystem,
		"horizontal_scaling_applied_replicas",
		[]string{"namespace", "target_name", "autoscaler_name", "source", le.JoinLeaderLabel},
		"Tracks the value of replicas applied by the horizontal scaling recommendation",
		commonOpts,
	)
	// telemetryHorizontalScaleConstraintsMinReplicas tracks the minReplicas constraint value
	telemetryHorizontalScaleConstraintsMinReplicas = telemetry.NewGaugeWithOpts(
		subsystem,
		"horizontal_scaling_constraints_min_replicas",
		dpaTelemetryLabels,
		"Tracks the minReplicas constraint value from DatadogPodAutoscaler.spec.constraints",
		commonOpts,
	)
	// telemetryHorizontalScaleConstraintsMaxReplicas tracks the maxReplicas constraint value
	telemetryHorizontalScaleConstraintsMaxReplicas = telemetry.NewGaugeWithOpts(
		subsystem,
		"horizontal_scaling_constraints_max_replicas",
		dpaTelemetryLabels,
		"Tracks the maxReplicas constraint value from DatadogPodAutoscaler.spec.constraints",
		commonOpts,
	)

	// telemetryVerticalScaleConstraintsContainerCPURequestMin tracks minimum CPU request constraint per container
	telemetryVerticalScaleConstraintsContainerCPURequestMin = telemetry.NewGaugeWithOpts(
		subsystem,
		"vertical_scaling_constraints_container_cpu_request_min",
		dpaSpecContainerTelemetryLabels,
		"Tracks minimum CPU request (in millicores) per container from DatadogPodAutoscaler.spec.constraints.containers.requests.minAllowed",
		commonOpts,
	)

	// telemetryVerticalScaleConstraintsContainerMemoryRequestMin tracks minimum memory request constraint per container
	telemetryVerticalScaleConstraintsContainerMemoryRequestMin = telemetry.NewGaugeWithOpts(
		subsystem,
		"vertical_scaling_constraints_container_memory_request_min",
		dpaSpecContainerTelemetryLabels,
		"Tracks minimum memory request (in bytes) per container from DatadogPodAutoscaler.spec.constraints.containers.requests.minAllowed",
		commonOpts,
	)

	// telemetryVerticalScaleConstraintsContainerCPURequestMax tracks maximum CPU request constraint per container
	telemetryVerticalScaleConstraintsContainerCPURequestMax = telemetry.NewGaugeWithOpts(
		subsystem,
		"vertical_scaling_constraints_container_cpu_request_max",
		dpaSpecContainerTelemetryLabels,
		"Tracks maximum CPU request (in millicores) per container from DatadogPodAutoscaler.spec.constraints.containers.requests.maxAllowed",
		commonOpts,
	)

	// telemetryVerticalScaleConstraintsContainerMemoryRequestMax tracks maximum memory request constraint per container
	telemetryVerticalScaleConstraintsContainerMemoryRequestMax = telemetry.NewGaugeWithOpts(
		subsystem,
		"vertical_scaling_constraints_container_memory_request_max",
		dpaSpecContainerTelemetryLabels,
		"Tracks maximum memory request (in bytes) per container from DatadogPodAutoscaler.spec.constraints.containers.requests.maxAllowed",
		commonOpts,
	)

	// telemetryStatusHorizontalCurrentReplicas tracks the current replicas value
	telemetryStatusHorizontalCurrentReplicas = telemetry.NewGaugeWithOpts(
		subsystem,
		"status_current_replicas",
		dpaTelemetryLabels,
		"Tracks the current replicas value from DatadogPodAutoscaler.status.currentReplicas",
		commonOpts,
	)

	// telemetryStatusHorizontalDesiredReplicas tracks the desired replicas value
	telemetryStatusHorizontalDesiredReplicas = telemetry.NewGaugeWithOpts(
		subsystem,
		"status_desired_replicas",
		dpaTelemetryLabels,
		"Tracks the current replicas value from DatadogPodAutoscaler.status.horizontal.desiredReplicas",
		commonOpts,
	)

	// telemetryStatusVerticalDesiredPodCPURequest tracks the sum of CPU requests for all containers
	telemetryStatusVerticalDesiredPodCPURequest = telemetry.NewGaugeWithOpts(
		subsystem,
		"status_vertical_desired_pod_cpu_request",
		dpaTelemetryLabels,
		"Tracks the sum of CPU requests (in millicores) from DatadogPodAutoscaler.status.vertical.target.podCPURequest",
		commonOpts,
	)

	// telemetryStatusVerticalDesiredPodMemoryRequest tracks the sum of memory requests for all containers
	telemetryStatusVerticalDesiredPodMemoryRequest = telemetry.NewGaugeWithOpts(
		subsystem,
		"status_vertical_desired_pod_memory_request",
		dpaTelemetryLabels,
		"Tracks the sum of memory requests (in bytes) from DatadogPodAutoscaler.status.vertical.target.podMemoryRequest",
		commonOpts,
	)

	// telemetryStatusVerticalDesiredContainerCPURequest tracks CPU requests per container
	telemetryStatusVerticalDesiredContainerCPURequest = telemetry.NewGaugeWithOpts(
		subsystem,
		"status_vertical_desired_container_cpu_request",
		dpaStatusContainerTelemetryLabels,
		"Tracks CPU requests (in millicores) per container from DatadogPodAutoscaler.status.vertical.target.desiredResources",
		commonOpts,
	)

	// telemetryStatusVerticalDesiredContainerMemoryRequest tracks memory requests per container
	telemetryStatusVerticalDesiredContainerMemoryRequest = telemetry.NewGaugeWithOpts(
		subsystem,
		"status_vertical_desired_container_memory_request",
		dpaStatusContainerTelemetryLabels,
		"Tracks memory requests (in bytes) per container from DatadogPodAutoscaler.status.vertical.target.desiredResources",
		commonOpts,
	)

	// telemetryStatusVerticalDesiredContainerCPULimit tracks CPU limits per container
	telemetryStatusVerticalDesiredContainerCPULimit = telemetry.NewGaugeWithOpts(
		subsystem,
		"status_vertical_desired_container_cpu_limit",
		dpaStatusContainerTelemetryLabels,
		"Tracks CPU limits (in millicores) per container from DatadogPodAutoscaler.status.vertical.target.desiredResources",
		commonOpts,
	)

	// telemetryStatusVerticalDesiredContainerMemoryLimit tracks memory limits per container
	telemetryStatusVerticalDesiredContainerMemoryLimit = telemetry.NewGaugeWithOpts(
		subsystem,
		"status_vertical_desired_container_memory_limit",
		dpaStatusContainerTelemetryLabels,
		"Tracks memory limits (in bytes) per container from DatadogPodAutoscaler.status.vertical.target.desiredResources",
		commonOpts,
	)

	// telemetryVerticalRolloutTriggered tracks the number of patch requests sent by the patcher to the kubernetes api server
	telemetryVerticalRolloutTriggered = telemetry.NewCounterWithOpts(
		subsystem,
		"vertical_rollout_triggered",
		[]string{"namespace", "target_name", "autoscaler_name", "status", le.JoinLeaderLabel},
		"Tracks the number of patch requests sent by the patcher to the kubernetes api server",
		commonOpts,
	)
	// telemetryVerticalScaleReceivedRecommendationsLimits tracks the vertical scaling recommendation limits received
	telemetryVerticalScaleReceivedRecommendationsLimits = telemetry.NewGaugeWithOpts(
		subsystem,
		"vertical_scaling_received_limits",
		dpaStatusContainerTelemetryLegacyLabels,
		"Tracks the value of limits received by the config retriever", commonOpts,
	)
	// telemetryVerticalScaleReceivedRecommendationsRequests tracks the vertical scaling recommendation requests received
	telemetryVerticalScaleReceivedRecommendationsRequests = telemetry.NewGaugeWithOpts(
		subsystem,
		"vertical_scaling_received_requests",
		dpaStatusContainerTelemetryLegacyLabels,
		"Tracks the value of requests received by the vertical scaling recommendation",
		commonOpts,
	)

	// autoscalingStatusConditions tracks the changes in autoscaler conditions
	autoscalingStatusConditions = telemetry.NewGaugeWithOpts(
		subsystem,
		"autoscaler_conditions",
		[]string{"namespace", "autoscaler_name", "type", le.JoinLeaderLabel},
		"Tracks the changes in autoscaler conditions",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)

	// telemetryLocalFallbackEnabled tracks whether local fallback recommendations are being used
	telemetryLocalFallbackEnabled = telemetry.NewGaugeWithOpts(
		subsystem,
		"local_fallback_enabled",
		dpaTelemetryLabels,
		"Tracks whether local fallback recommendations are being used",
		commonOpts,
	)

	// telemetryMetricsForDeletion contains all gauge metrics that need to be cleaned up when deleting pod autoscaler telemetry
	telemetryMetricsForDeletion = []telemetry.Gauge{
		telemetryReceivedRecommendationsVersion,
		telemetryHorizontalScaleAppliedRecommendations,
		telemetryHorizontalScaleReceivedRecommendations,
		telemetryHorizontalScaleConstraintsMinReplicas,
		telemetryHorizontalScaleConstraintsMaxReplicas,
		telemetryVerticalScaleConstraintsContainerCPURequestMin,
		telemetryVerticalScaleConstraintsContainerMemoryRequestMin,
		telemetryVerticalScaleConstraintsContainerCPURequestMax,
		telemetryVerticalScaleConstraintsContainerMemoryRequestMax,
		telemetryVerticalScaleReceivedRecommendationsLimits,
		telemetryVerticalScaleReceivedRecommendationsRequests,
		autoscalingStatusConditions,
		telemetryStatusHorizontalCurrentReplicas,
		telemetryStatusHorizontalDesiredReplicas,
		telemetryStatusVerticalDesiredPodCPURequest,
		telemetryStatusVerticalDesiredPodMemoryRequest,
		telemetryStatusVerticalDesiredContainerCPURequest,
		telemetryStatusVerticalDesiredContainerMemoryRequest,
		telemetryStatusVerticalDesiredContainerCPULimit,
		telemetryStatusVerticalDesiredContainerMemoryLimit,
	}
)

func trackPodAutoscalerReceivedValues(podAutoscaler model.PodAutoscalerInternal, version uint64) {
	// Emit telemetry for received values
	// Target name cannot normally be empty, but we handle it just in case
	var targetName string
	if podAutoscaler.Spec() != nil {
		targetName = podAutoscaler.Spec().TargetRef.Name
	}

	scalingValues := podAutoscaler.MainScalingValues()

	// Track received recommendations version
	telemetryReceivedRecommendationsVersion.Set(
		float64(version),
		podAutoscaler.Namespace(),
		targetName,
		podAutoscaler.Name(),
		le.JoinLeaderValue,
	)

	// Horizontal value
	if podAutoscaler.MainScalingValues().Horizontal != nil {
		telemetryHorizontalScaleReceivedRecommendations.Set(
			float64(scalingValues.Horizontal.Replicas),
			podAutoscaler.Namespace(),
			targetName,
			podAutoscaler.Name(),
			string(scalingValues.Horizontal.Source),
			le.JoinLeaderValue,
		)
	}

	// Vertical values
	if scalingValues.Vertical != nil {
		for _, containerResources := range scalingValues.Vertical.ContainerResources {
			for resource, value := range containerResources.Requests {
				telemetryVerticalScaleReceivedRecommendationsRequests.Set(
					value.AsApproximateFloat64(),
					podAutoscaler.Namespace(),
					targetName,
					podAutoscaler.Name(),
					string(scalingValues.Vertical.Source),
					containerResources.Name,
					containerResources.Name,
					string(resource),
					le.JoinLeaderValue,
				)
			}

			for resource, value := range containerResources.Limits {
				telemetryVerticalScaleReceivedRecommendationsLimits.Set(
					value.AsApproximateFloat64(),
					podAutoscaler.Namespace(),
					targetName,
					podAutoscaler.Name(),
					string(scalingValues.Vertical.Source),
					containerResources.Name,
					containerResources.Name,
					string(resource),
					le.JoinLeaderValue,
				)
			}
		}
	}
}

func trackPodAutoscalerStatus(podAutoscaler *datadoghq.DatadogPodAutoscaler) {
	for _, condition := range podAutoscaler.Status.Conditions {
		if condition.Status == corev1.ConditionTrue {
			autoscalingStatusConditions.Set(1.0, podAutoscaler.Namespace, podAutoscaler.Name, string(condition.Type), le.JoinLeaderValue)
		} else {
			autoscalingStatusConditions.Set(0.0, podAutoscaler.Namespace, podAutoscaler.Name, string(condition.Type), le.JoinLeaderValue)
		}
	}
}

func deletePodAutoscalerTelemetry(ns, autoscalerName string) {
	log.Debugf("Deleting pod autoscaler telemetry for %s/%s", ns, autoscalerName)
	tags := map[string]string{
		"namespace":        ns,
		"autoscaler_name":  autoscalerName,
		le.JoinLeaderLabel: le.JoinLeaderValue,
	}

	for _, metric := range telemetryMetricsForDeletion {
		metric.DeletePartialMatch(tags)
	}
}

func trackLocalFallbackEnabled(currentSource datadoghqcommon.DatadogPodAutoscalerValueSource, podAutoscalerInternal model.PodAutoscalerInternal) {
	labels := getAutoscalerTelemetryLabels(podAutoscalerInternal)

	var value float64
	if currentSource == datadoghqcommon.DatadogPodAutoscalerLocalValueSource {
		value = 1
	} else {
		value = 0
	}
	telemetryLocalFallbackEnabled.Set(value, labels...)
}

func trackDPATelemetry(podAutoscaler *datadoghq.DatadogPodAutoscaler) {
	trackHorizontalConstraints(podAutoscaler)
	trackVerticalConstraints(podAutoscaler)
	trackHorizontalStatus(podAutoscaler)
	trackVerticalStatus(podAutoscaler)
}

func trackHorizontalConstraints(podAutoscaler *datadoghq.DatadogPodAutoscaler) {
	labels := getAutoscalerTelemetryLabels(podAutoscaler)
	spec := podAutoscaler.Spec

	// Track minReplicas or delete if not set
	var minReplicas *int32
	if spec.Constraints != nil {
		minReplicas = spec.Constraints.MinReplicas
	}
	setOrDeleteGauge(telemetryHorizontalScaleConstraintsMinReplicas, minReplicas, labels...)

	// Track maxReplicas or delete if not set
	if spec.Constraints != nil && spec.Constraints.MaxReplicas > 0 {
		maxReplicas := spec.Constraints.MaxReplicas
		telemetryHorizontalScaleConstraintsMaxReplicas.Set(float64(maxReplicas), labels...)
	} else {
		telemetryHorizontalScaleConstraintsMaxReplicas.Delete(labels...)
	}
}

func trackVerticalConstraints(podAutoscaler *datadoghq.DatadogPodAutoscaler) {
	labels := getAutoscalerTelemetryLabels(podAutoscaler)
	spec := podAutoscaler.Spec

	// Always delete all container constraint metrics and then recreate them
	// This is needed because we can't know when a container has been removed from the constraints
	deleteGaugeLinkedToDPA(telemetryVerticalScaleConstraintsContainerCPURequestMin, labels[0], labels[2])
	deleteGaugeLinkedToDPA(telemetryVerticalScaleConstraintsContainerMemoryRequestMin, labels[0], labels[2])
	deleteGaugeLinkedToDPA(telemetryVerticalScaleConstraintsContainerCPURequestMax, labels[0], labels[2])
	deleteGaugeLinkedToDPA(telemetryVerticalScaleConstraintsContainerMemoryRequestMax, labels[0], labels[2])

	if spec.Constraints != nil && len(spec.Constraints.Containers) > 0 {
		// Track per-container constraints
		for _, containerConstraint := range spec.Constraints.Containers {
			containerName := allContainerLabelContainerNameValue
			if containerConstraint.Name != "" {
				containerName = containerConstraint.Name
			}
			containerLabels := append(labels[:len(labels)-1], containerName, le.JoinLeaderValue)

			// Track request constraints if present
			if containerConstraint.Requests != nil {
				// Track min constraints
				trackContainerResource(telemetryVerticalScaleConstraintsContainerCPURequestMin, containerConstraint.Requests.MinAllowed, corev1.ResourceCPU, true, containerLabels...)
				trackContainerResource(telemetryVerticalScaleConstraintsContainerMemoryRequestMin, containerConstraint.Requests.MinAllowed, corev1.ResourceMemory, false, containerLabels...)

				// Track max constraints
				trackContainerResource(telemetryVerticalScaleConstraintsContainerCPURequestMax, containerConstraint.Requests.MaxAllowed, corev1.ResourceCPU, true, containerLabels...)
				trackContainerResource(telemetryVerticalScaleConstraintsContainerMemoryRequestMax, containerConstraint.Requests.MaxAllowed, corev1.ResourceMemory, false, containerLabels...)
			}
		}
	}
}

func trackHorizontalStatus(podAutoscaler *datadoghq.DatadogPodAutoscaler) {
	labels := getAutoscalerTelemetryLabels(podAutoscaler)

	// Track current replicas
	setOrDeleteGauge(telemetryStatusHorizontalCurrentReplicas, podAutoscaler.Status.CurrentReplicas, labels...)

	// Track desired replicas
	if podAutoscaler.Status.Horizontal != nil {
		desiredReplicas := podAutoscaler.Status.Horizontal.Target.Replicas
		telemetryStatusHorizontalDesiredReplicas.Set(float64(desiredReplicas), labels...)
	} else {
		telemetryStatusHorizontalDesiredReplicas.Delete(labels...)
	}
}

func trackVerticalStatus(podAutoscaler *datadoghq.DatadogPodAutoscaler) {
	labels := getAutoscalerTelemetryLabels(podAutoscaler)

	// Always delete all container-specific metrics before recreating them
	// This is needed because we can't know when a container has been removed from the status
	deleteGaugeLinkedToDPA(telemetryStatusVerticalDesiredContainerCPURequest, labels[0], labels[2])
	deleteGaugeLinkedToDPA(telemetryStatusVerticalDesiredContainerMemoryRequest, labels[0], labels[2])
	deleteGaugeLinkedToDPA(telemetryStatusVerticalDesiredContainerCPULimit, labels[0], labels[2])
	deleteGaugeLinkedToDPA(telemetryStatusVerticalDesiredContainerMemoryLimit, labels[0], labels[2])

	if podAutoscaler.Status.Vertical != nil && podAutoscaler.Status.Vertical.Target != nil {
		target := podAutoscaler.Status.Vertical.Target
		source := string(target.Source)

		// Track pod-level CPU and memory requests
		telemetryStatusVerticalDesiredPodCPURequest.Set(float64(target.PodCPURequest.MilliValue()), labels...)
		telemetryStatusVerticalDesiredPodMemoryRequest.Set(float64(target.PodMemoryRequest.Value()), labels...)

		// Track per-container resources
		for _, containerResources := range target.DesiredResources {
			containerLabels := append(labels[:len(labels)-1], source, containerResources.Name, le.JoinLeaderValue)
			trackContainerResources(containerResources, containerLabels)
		}
	} else {
		// Delete pod-level metrics if vertical status is not set
		telemetryStatusVerticalDesiredPodCPURequest.Delete(labels...)
		telemetryStatusVerticalDesiredPodMemoryRequest.Delete(labels...)
	}
}

// trackContainerResources tracks all resources (requests and limits) for a single container
func trackContainerResources(containerResources datadoghqcommon.DatadogPodAutoscalerContainerResources, containerLabels []string) {
	// Track CPU request
	trackContainerResource(telemetryStatusVerticalDesiredContainerCPURequest, containerResources.Requests, corev1.ResourceCPU, true, containerLabels...)

	// Track memory request
	trackContainerResource(telemetryStatusVerticalDesiredContainerMemoryRequest, containerResources.Requests, corev1.ResourceMemory, false, containerLabels...)

	// Track CPU limit (optional)
	trackContainerResource(telemetryStatusVerticalDesiredContainerCPULimit, containerResources.Limits, corev1.ResourceCPU, true, containerLabels...)

	// Track memory limit (optional)
	trackContainerResource(telemetryStatusVerticalDesiredContainerMemoryLimit, containerResources.Limits, corev1.ResourceMemory, false, containerLabels...)
}

// trackContainerResource tracks a specific resource (CPU or memory) with optional delete if not present
func trackContainerResource(metric telemetry.Gauge, resourceList corev1.ResourceList, resourceName corev1.ResourceName, isMilliValue bool, labels ...string) {
	if quantity, ok := resourceList[resourceName]; ok {
		var value float64
		if isMilliValue {
			value = float64(quantity.MilliValue())
		} else {
			value = float64(quantity.Value())
		}
		metric.Set(value, labels...)
	} else {
		metric.Delete(labels...)
	}
}

func setHorizontalScaleAppliedRecommendations(toReplicas float64, ns, targetName, autoscalerName, source string) {
	// Clear previous values to prevent gauge from reporting old values for different sources
	deleteGaugeLinkedToDPA(telemetryHorizontalScaleAppliedRecommendations, ns, autoscalerName)

	telemetryHorizontalScaleAppliedRecommendations.Set(
		toReplicas,
		ns,
		targetName,
		autoscalerName,
		source,
		le.JoinLeaderValue,
	)
}

func deleteGaugeLinkedToDPA(metric telemetry.Gauge, ns, autoscalerName string) {
	tags := map[string]string{
		"namespace":       ns,
		"autoscaler_name": autoscalerName,
	}

	metric.DeletePartialMatch(tags)
}

// getAutoscalerTelemetryLabels extracts common labels from a DatadogPodAutoscaler or PodAutoscalerInternal
func getAutoscalerTelemetryLabels[T *datadoghq.DatadogPodAutoscaler | model.PodAutoscalerInternal](autoscaler T) []string {
	var namespace, targetName, name string

	// TODO the internal model can be improve t
	switch a := any(autoscaler).(type) {
	case *datadoghq.DatadogPodAutoscaler:
		namespace = a.Namespace
		targetName = a.Spec.TargetRef.Name
		name = a.Name
	case model.PodAutoscalerInternal:
		namespace = a.Namespace()
		targetName = a.Spec().TargetRef.Name
		name = a.Name()
	}

	return []string{
		namespace,
		targetName,
		name,
		le.JoinLeaderValue,
	}
}

// setOrDeleteGauge sets a gauge metric with the provided value if it's not nil, otherwise deletes it
func setOrDeleteGauge(metric telemetry.Gauge, value *int32, labels ...string) {
	if value != nil {
		metric.Set(float64(*value), labels...)
	} else {
		metric.Delete(labels...)
	}
}
