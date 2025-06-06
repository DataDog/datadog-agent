// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package local provides local recommendations for autoscaling workloads.
package local

import (
	"fmt"
	"math"
	"time"

	"k8s.io/utils/clock"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/loadstore"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	le "github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver/leaderelection/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	minRequiredMetricDataPoints = 2 // minimum number of data points to consider for a metric
)

type replicaCalculator struct {
	podWatcher workload.PodWatcher
	clock      clock.Clock
}

type utilizationResult struct {
	averageUtilization      float64
	podToUtilization        map[string]float64
	missingPods             []string
	recommendationTimestamp time.Time
}

func newReplicaCalculator(clock clock.Clock, podWatcher workload.PodWatcher) replicaCalculator {
	return replicaCalculator{
		podWatcher: podWatcher,
		clock:      clock,
	}
}

// calculateHorizontalRecommendations is the entrypoint to calculate the horizontal recommendation for a given DatadogPodAutoscaler
func (r replicaCalculator) calculateHorizontalRecommendations(dpai model.PodAutoscalerInternal, lStore loadstore.Store) (*model.HorizontalScalingValues, error) {
	currentTime := r.clock.Now()

	// Get current pods for the target
	targetRef := dpai.Spec().TargetRef
	objectives := dpai.Spec().Objectives
	targetGVK, targetErr := dpai.TargetGVK()
	if targetErr != nil {
		return nil, fmt.Errorf("Failed to get GVK for target: %s, %s", dpai.ID(), targetErr)
	}

	podOwnerName := targetRef.Name
	namespace := dpai.Namespace()

	podOwner := workload.NamespacedPodOwner{
		Namespace: namespace,
		Name:      podOwnerName,
		Kind:      targetGVK.Kind,
	}
	pods := r.podWatcher.GetPodsForOwner(podOwner)
	if len(pods) == 0 {
		// If we found nothing, we'll wait just until the next sync
		return nil, fmt.Errorf("No pods found for autoscaler: %s, gvk: %s, name: %s", dpai.ID(), targetGVK.String(), targetRef.Name)
	}

	recommendedReplicas := model.HorizontalScalingValues{}

	for _, objective := range objectives {
		recSettings, err := newResourceRecommenderSettings(objective)
		if err != nil {
			return nil, fmt.Errorf("Failed to get resource recommender settings: %s", err)
		}

		queryResult := lStore.GetMetricsRaw(recSettings.metricName, namespace, podOwnerName, recSettings.containerName)
		rec, utilizationRes, err := recommend(currentTime, *recSettings, pods, queryResult)
		if err != nil {
			log.Debugf("Got error calculating recommendation: %s", err)
			break
		}
		ts := utilizationRes.recommendationTimestamp

		telemetryHorizontalLocalUtilizationPct.Set(
			float64(utilizationRes.averageUtilization),
			namespace,
			podOwnerName,
			dpai.Name(),
			string(recommendedReplicas.Source),
			le.JoinLeaderValue,
		)

		// Always choose the highest recommendation given
		if rec > recommendedReplicas.Replicas {
			recommendedReplicas.Replicas = rec
			recommendedReplicas.Timestamp = ts
			recommendedReplicas.Source = datadoghqcommon.DatadogPodAutoscalerLocalValueSource
		}
	}

	if recommendedReplicas.Replicas == 0 {
		return nil, fmt.Errorf("No recommendation found for autoscaler: %s", dpai.ID())
	}

	telemetryHorizontalLocalRecommendations.Set(
		float64(recommendedReplicas.Replicas),
		namespace,
		podOwnerName,
		dpai.Name(),
		string(recommendedReplicas.Source),
		le.JoinLeaderValue,
	)

	return &recommendedReplicas, nil
}

func recommend(currentTime time.Time, recSettings resourceRecommenderSettings, pods []*workloadmeta.KubernetesPod, queryResult loadstore.QueryResult) (int32, utilizationResult, error) {
	currentReplicas := float64(len(pods))
	utilizationRes, err := calculateUtilization(recSettings, pods, queryResult, currentTime)
	if err != nil {
		return 0, utilizationResult{}, err
	}

	recommendedReplicas := calculateReplicas(recSettings, currentReplicas, utilizationRes.averageUtilization)

	scaleDirection := common.GetScaleDirection(int32(currentReplicas), recommendedReplicas)
	if scaleDirection != common.NoScale && len(utilizationRes.missingPods) > 0 {
		adjustedPodToUtilization := adjustMissingPods(scaleDirection, utilizationRes.podToUtilization, utilizationRes.missingPods)
		adjustedUtilization := getAveragePodUtilization(adjustedPodToUtilization)
		newRecommendation := calculateReplicas(recSettings, currentReplicas, adjustedUtilization)

		// If scale direction is reversed, we should not scale
		if common.GetScaleDirection(int32(currentReplicas), newRecommendation) != scaleDirection {
			recommendedReplicas = int32(currentReplicas)
		} else {
			recommendedReplicas = newRecommendation
			utilizationRes.averageUtilization = adjustedUtilization
		}
	}

	return recommendedReplicas, utilizationRes, nil
}

func calculateUtilization(recSettings resourceRecommenderSettings, pods []*workloadmeta.KubernetesPod, queryResult loadstore.QueryResult, currentTime time.Time) (utilizationResult, error) {
	totalPodUtilization := 0.0
	podCount := 0
	podUtilization := make(map[string]float64)
	missingPods := []string{}
	lastValidTimestamp := time.Time{}

	if len(pods) == 0 {
		return utilizationResult{}, fmt.Errorf("No pods found")
	}

	if len(queryResult.Results) == 0 {
		return utilizationResult{}, fmt.Errorf("Issue fetching metrics data")
	}

	for _, pod := range pods {
		totalUsage := 0.0
		totalRequests := 0.0

		for _, container := range pod.Containers {
			if recSettings.containerName != "" && container.Name != recSettings.containerName {
				continue
			}

			if recSettings.metricName == "container.memory.usage" && container.Resources.MemoryRequest != nil {
				totalRequests += float64(*container.Resources.MemoryRequest)
			} else if recSettings.metricName == "container.cpu.usage" && container.Resources.CPURequest != nil {
				totalRequests += convertCPURequestToNanocores(*container.Resources.CPURequest)
			} else {
				continue // skip; no request information
			}

			series := getContainerMetrics(queryResult, pod.Name, container.Name)
			averageValue, lastTimestamp, err := processAverageContainerMetricValue(series, currentTime, recSettings.fallbackStaleDataThreshold)
			if err != nil {
				continue // skip; no usage information
			}
			totalUsage += averageValue
			if lastTimestamp.After(lastValidTimestamp) {
				lastValidTimestamp = lastTimestamp
			}
		}

		if totalRequests > 0 && totalUsage > 0 {
			utilization := totalUsage / totalRequests
			podUtilization[pod.Name] = utilization
			totalPodUtilization += podUtilization[pod.Name]
			podCount++
		} else {
			missingPods = append(missingPods, pod.Name)
		}
	}

	if podCount == 0 {
		return utilizationResult{}, fmt.Errorf("Issue calculating pod utilization")
	}

	return utilizationResult{
		averageUtilization:      totalPodUtilization / float64(podCount),
		missingPods:             missingPods,
		podToUtilization:        podUtilization,
		recommendationTimestamp: lastValidTimestamp,
	}, nil
}

func getAveragePodUtilization(podToUtilization map[string]float64) float64 {
	totalUtilization := 0.0
	for _, utilization := range podToUtilization {
		totalUtilization += utilization
	}
	return totalUtilization / float64(len(podToUtilization))
}

// getContainerMetrics retrieves the metrics for a specific container in a pod
func getContainerMetrics(queryResult loadstore.QueryResult, podName, containerName string) []loadstore.EntityValue {
	for _, result := range queryResult.Results {
		if result.PodName == podName {
			if series, ok := result.ContainerValues[containerName]; ok {
				return series
			}
		}
	}
	return nil
}

// processAverageContainerMetricValue takes a series of metrics and processes them to return the final metric value and
// corresponding timestamp to use to generate a recommendation
func processAverageContainerMetricValue(series []loadstore.EntityValue, currentTime time.Time, fallbackStaleDataThreshold int64) (float64, time.Time, error) {
	if len(series) < 2 { // too little metrics data
		return 0.0, time.Time{}, fmt.Errorf("Missing usage metrics")
	}

	values := []loadstore.ValueType{}
	var lastTimestamp time.Time

	// series is already sorted oldest to newest data point based on insertion
	for i := len(series) - 1; i >= 0; i-- {
		entity := series[i]
		// Invalid data point; data not yet populated
		if entity.Timestamp == 0 {
			continue
		}

		// Discard stale metrics
		if isStaleMetric(currentTime, entity.Timestamp, fallbackStaleDataThreshold) && len(values) >= minRequiredMetricDataPoints {
			continue
		}

		values = append(values, entity.Value)
		ts := convertTimestampToTime(entity.Timestamp)
		// we want to keep the oldest timestamp used in the average
		if (lastTimestamp == time.Time{}) || ts.Before(lastTimestamp) {
			lastTimestamp = ts
		}

	}

	return average(values), lastTimestamp, nil
}

func adjustMissingPods(scaleDirection common.ScaleDirection, podToUtilization map[string]float64, missingPods []string) map[string]float64 {
	for _, pod := range missingPods {
		// adjust based on scale direction
		if scaleDirection == common.ScaleUp {
			podToUtilization[pod] = 0.0 // 0%
		} else if scaleDirection == common.ScaleDown {
			podToUtilization[pod] = 1.0 // 100%
		}
	}
	return podToUtilization
}

func calculateReplicas(recSettings resourceRecommenderSettings, currentReplicas float64, averageUtilization float64) int32 {
	recommendedReplicas := int32(currentReplicas)

	if averageUtilization > recSettings.highWatermark {
		rec := int32(math.Ceil(averageUtilization / recSettings.highWatermark * currentReplicas))

		if rec > recommendedReplicas {
			recommendedReplicas = rec
		}
	}

	if averageUtilization < recSettings.lowWatermark {
		proposedReplicas := math.Max(math.Floor(averageUtilization/recSettings.lowWatermark*currentReplicas), 1)

		// Adjust to be below the high watermark
		for ; proposedReplicas < currentReplicas; proposedReplicas++ {
			forecastValue := (currentReplicas * averageUtilization / proposedReplicas)

			// Only allow if we don't break the high watermark
			if forecastValue < recSettings.highWatermark {
				recommendedReplicas = int32(proposedReplicas)
				break
			}
		}
	}

	return recommendedReplicas
}

// Helpers
func isStaleMetric(currentTime time.Time, metricTimestamp loadstore.Timestamp, staleDataThresholdSeconds int64) bool {
	return currentTime.Unix()-int64(metricTimestamp) > staleDataThresholdSeconds
}

func average(series []loadstore.ValueType) float64 {
	average := loadstore.ValueType(0)
	for _, val := range series {
		average += val
	}
	average /= loadstore.ValueType(len(series))
	return float64(average)
}

func convertTimestampToTime(timestamp loadstore.Timestamp) time.Time {
	return time.Unix(int64(timestamp), 0)
}

func convertCPURequestToNanocores(cpuRequests float64) float64 {
	// Current implementation takes Mi value and returns .AsApproximateFloat64()*100
	// For 100m, AsApproximate returns 0.1; we return 10%
	// This helper converts value to nanocore units (default from loadstore)
	return (cpuRequests / 100) * 1e9
}
