// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

// Package local provides local recommendations for autoscaling workloads.
package local

import (
	"fmt"
	"math"
	"time"

	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	corev1 "k8s.io/api/core/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"

	// "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/loadstore"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/shared"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type LocalRecommender struct {
	PodWatcher shared.PodWatcher
	// Store     *loadstore.EntityStore
}

type ValueType float64
type Timestamp uint32

// EntityValue represents a value with a timestamp.
type EntityValue struct {
	value     ValueType
	timestamp Timestamp
}

// Structs from loadstore implementation //
type PodResult struct {
	PodName         string
	ContainerValues map[string][]EntityValue // container name to a time series of entity values, e.g cpu usage from past three collection
	PodLevelValue   []EntityValue            //  If Pod level value is not available, it will be empty
}

type QueryResult struct { // return value for GetMetricsRaw
	results []PodResult
}

const (
	staleDataThresholdSeconds = 180 // 3 minutes
)

var (
	resourceToMetric = map[corev1.ResourceName]string{
		corev1.ResourceCPU:    "container.cpu.usage",
		corev1.ResourceMemory: "container.memory.usage",
	}

	// Example data
	ev1 = EntityValue{value: 0.1, timestamp: 1}
	ev2 = EntityValue{value: 0.2, timestamp: 2}
	ev3 = EntityValue{value: 0.3, timestamp: 3}
	ev4 = EntityValue{value: 0.4, timestamp: 4}
	ev5 = EntityValue{value: 0.5, timestamp: 5}

	// Sample PodResult
	podResult1 = PodResult{
		PodName: "podName-1",
		ContainerValues: map[string][]EntityValue{
			"containerName1": {ev1, ev2, ev3},
			"containerName2": {ev4, ev5},
		},
	}

	podResult2 = PodResult{
		PodName: "podName-2",
		ContainerValues: map[string][]EntityValue{
			"containerName3": {ev1, ev2, ev3},
		},
		PodLevelValue: []EntityValue{ev1, ev2, ev3},
	}

	// Sample QueryResult
	queryResult = QueryResult{
		results: []PodResult{podResult1, podResult2},
	}

	queryResultContainerFiltered = QueryResult{
		results: []PodResult{
			{
				PodName: "podName-2",
				ContainerValues: map[string][]EntityValue{
					"containerName1": {ev1, ev2, ev3},
				},
			},
		},
	}
)

type resourceRecommenderSettings struct {
	MetricName    string
	ContainerName *string
	LowWatermark  float64
	HighWatermark float64
}

type UtilizationResult struct {
	AverageUtilization      float64
	PodToUtilization        map[string]float64
	MissingPods             []string
	RecommendationTimestamp time.Time
}

func newResourceRecommenderSettings(target datadoghq.DatadogPodAutoscalerTarget) (*resourceRecommenderSettings, error) {
	if target.Type == datadoghq.DatadogPodAutoscalerContainerResourceTargetType {
		return getOptionsFromContainerResource(target.ContainerResource)
	}
	if target.Type == datadoghq.DatadogPodAutoscalerResourceTargetType {
		return getOptionsFromPodResource(target.PodResource)
	}
	return nil, fmt.Errorf("Invalid target type: %s", target.Type)
}

func getOptionsFromPodResource(target *datadoghq.DatadogPodAutoscalerResourceTarget) (*resourceRecommenderSettings, error) {
	if target == nil {
		return nil, fmt.Errorf("nil target")
	}
	if target.Value.Type != datadoghq.DatadogPodAutoscalerUtilizationTargetValueType {
		return nil, fmt.Errorf("invalid value type: %s", target.Value.Type)
	}
	metric, ok := resourceToMetric[target.Name]
	if !ok {
		return nil, fmt.Errorf("invalid resource name: %s", target.Name)
	}

	recSettings := &resourceRecommenderSettings{
		MetricName:    metric,
		LowWatermark:  float64((*target.Value.Utilization - 5)) / 100.0,
		HighWatermark: float64((*target.Value.Utilization + 5)) / 100.0,
	}
	return recSettings, nil
}

func getOptionsFromContainerResource(target *datadoghq.DatadogPodAutoscalerContainerResourceTarget) (*resourceRecommenderSettings, error) {
	if target == nil {
		return nil, fmt.Errorf("nil target")
	}
	if target.Value.Type != datadoghq.DatadogPodAutoscalerUtilizationTargetValueType {
		return nil, fmt.Errorf("invalid value type: %s", target.Value.Type)
	}

	metric, ok := resourceToMetric[target.Name]
	if !ok {
		return nil, fmt.Errorf("invalid resource name: %s", target.Name)
	}

	recSettings := &resourceRecommenderSettings{
		MetricName:    metric,
		LowWatermark:  float64((*target.Value.Utilization - 5)) / 100.0,
		HighWatermark: float64((*target.Value.Utilization + 5)) / 100.0,
	}
	return recSettings, nil
}

// CalculateHorizontalRecommendations is the entrypoint to calculate the horizontal recommendation for a given DatadogPodAutoscaler
func (l LocalRecommender) CalculateHorizontalRecommendations(dpai model.PodAutoscalerInternal) (*model.ScalingValues, error) {
	currentTime := time.Now()

	// Get current pods for the target
	targetRef := dpai.Spec().TargetRef
	targets := dpai.Spec().Targets
	targetGVK, targetErr := dpai.TargetGVK()
	if targetErr != nil {
		return nil, fmt.Errorf("Failed to get GVK for target: %s, %s", dpai.ID(), targetErr)
	}
	podOwner := shared.NamespacedPodOwner{
		Namespace: dpai.Namespace(),
		Name:      targetRef.Name,
		Kind:      targetGVK.Kind,
	}
	pods := l.PodWatcher.GetPodsForOwner(podOwner)
	if len(pods) == 0 {
		// If we found nothing, we'll wait just until the next sync
		return nil, fmt.Errorf("No pods found for autoscaler: %s, gvk: %s, name: %s", dpai.ID(), targetGVK.String(), targetRef.Name)
	}

	recommendedReplicas := model.HorizontalScalingValues{}

	for _, target := range targets {
		recSettings, err := newResourceRecommenderSettings(target)
		if err != nil {
			return nil, fmt.Errorf("Failed to get resource recommender settings: %s", err)
		}

		// queryResult := GetMetricsRaw()
		rec, ts, err := recSettings.recommend(currentTime, pods, queryResult, float64(*dpai.CurrentReplicas()))
		if err != nil {
			log.Debugf("Got error calculating recommendation: %s", err)
			break
		}

		// always choose the highest recommendation given
		if rec > recommendedReplicas.Replicas {
			recommendedReplicas.Replicas = rec
			recommendedReplicas.Timestamp = ts
			recommendedReplicas.Source = datadoghq.DatadogPodAutoscalerLocalValueSource
		}
	}

	// TODO: need error handling in caller to check err and not to update (we don't want to recommend 0)
	if recommendedReplicas.Replicas == 0 {
		return nil, fmt.Errorf("No recommendation found for autoscaler: %s", dpai.ID())
	}

	return &model.ScalingValues{
		Horizontal: &recommendedReplicas,
	}, nil
}

func (r resourceRecommenderSettings) recommend(currentTime time.Time, pods []*workloadmeta.KubernetesPod, queryResult QueryResult, currentReplicas float64) (int32, time.Time, error) {
	utilizationResult, err := r.calculateUtilization(pods, queryResult, currentTime)
	if err != nil {
		return 0, time.Time{}, err
	}

	recommendedReplicas := r.calculateReplicas(currentReplicas, utilizationResult.AverageUtilization)

	scaleDirection := shared.GetScaleDirection(int32(currentReplicas), recommendedReplicas)
	// account for missing pods
	if scaleDirection != shared.NoScale && len(utilizationResult.MissingPods) > 0 {
		adjustedPodToUtilization := adjustMissingPods(scaleDirection, utilizationResult.PodToUtilization, utilizationResult.MissingPods)
		newRecommendation := r.calculateReplicas(currentReplicas, getAveragePodUtilization(adjustedPodToUtilization))

		// If scale direction is reversed, we should not scale
		if shared.GetScaleDirection(int32(currentReplicas), newRecommendation) != scaleDirection {
			recommendedReplicas = int32(currentReplicas)
		} else {
			recommendedReplicas = newRecommendation
		}
	}

	return recommendedReplicas, utilizationResult.RecommendationTimestamp, nil
}

func (r *resourceRecommenderSettings) calculateUtilization(pods []*workloadmeta.KubernetesPod, queryResult QueryResult, currentTime time.Time) (UtilizationResult, error) {
	totalPodUtilization := 0.0
	podCount := 0
	podUtilization := make(map[string]float64)
	missingPods := []string{}
	lastValidTimestamp := time.Time{}

	if len(pods) == 0 {
		return UtilizationResult{}, fmt.Errorf("No pods found")
	}

	if len(queryResult.results) == 0 {
		return UtilizationResult{}, fmt.Errorf("Issue fetching metrics data")
	}

	for _, pod := range pods {
		totalUsage := 0.0
		totalRequests := 0.0

		for _, container := range pod.Containers {
			if r.ContainerName != nil && container.Name != *r.ContainerName {
				continue
			}

			if r.MetricName == "container.memory.usage" && container.Resources.MemoryRequest != nil {
				totalRequests += float64(*container.Resources.MemoryRequest)
			} else if r.MetricName == "container.cpu.usage" && container.Resources.CPURequest != nil {
				totalRequests += convertCpuRequestToMillicores(*container.Resources.CPURequest)
			} else {
				continue // skip; no request information
			}

			series := getContainerMetrics(queryResult, pod.Name, container.Name)
			if len(series) == 0 { // no metrics data
				continue
			}

			averageValue, lastTimestamp, err := processAverageContainerMetricValue(series, currentTime)
			if err != nil {
				continue // skip; no usage information
			}
			totalUsage += averageValue
			if lastTimestamp.After(lastValidTimestamp) {
				lastValidTimestamp = lastTimestamp
			}
		}

		if totalRequests > 0 && totalUsage > 0 {
			podUtilization[pod.Name] = totalUsage / totalRequests
			totalPodUtilization += podUtilization[pod.Name]
			podCount++
		} else {
			missingPods = append(missingPods, pod.Name)
		}
	}

	if podCount == 0 {
		return UtilizationResult{}, fmt.Errorf("Issue calculating pod utilization")
	}

	return UtilizationResult{
		AverageUtilization:      totalPodUtilization / float64(podCount),
		MissingPods:             missingPods,
		PodToUtilization:        podUtilization,
		RecommendationTimestamp: lastValidTimestamp,
	}, nil
}

func getAveragePodUtilization(podToUtilization map[string]float64) float64 {
	totalUtilization := 0.0
	for _, utilization := range podToUtilization {
		totalUtilization += utilization
	}
	return totalUtilization / float64(len(podToUtilization))
}

// GetContainerMetrics retrieves the metrics for a specific container in a pod
func getContainerMetrics(queryResult QueryResult, podName, containerName string) []EntityValue {
	for _, result := range queryResult.results {
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
// TODO: handle missing containers here?
func processAverageContainerMetricValue(series []EntityValue, currentTime time.Time) (float64, time.Time, error) {
	values := []ValueType{}
	lastTimestamp := time.Time{}

	for _, entity := range series {
		// Discard stale metrics
		if isStaleMetric(currentTime, entity.timestamp) {
			continue
		}
		values = append(values, entity.value)
		ts := convertTimestampToTime(entity.timestamp)
		if ts.After(lastTimestamp) {
			lastTimestamp = ts
		}
	}

	if len(values) == 0 {
		return 0.0, lastTimestamp, fmt.Errorf("Missing usage metrics")
	}

	return average(values), lastTimestamp, nil
}

func findAverageUtilization(podToUtilization map[string]float64) float64 {
	totalUtilization := 0.0
	for _, utilization := range podToUtilization {
		totalUtilization += utilization
	}
	return totalUtilization / float64(len(podToUtilization))
}

func adjustMissingPods(scaleDirection shared.ScaleDirection, podToUtilization map[string]float64, missingPods []string) map[string]float64 {
	for _, pod := range missingPods {
		// adjust based on scale direction
		if scaleDirection == shared.ScaleUp {
			podToUtilization[pod] = 0.0 // 0%
		} else if scaleDirection == shared.ScaleDown {
			podToUtilization[pod] = 1.0 // 100%
		}
	}
	return podToUtilization
}

func (r *resourceRecommenderSettings) calculateReplicas(currentReplicas float64, averageUtilization float64) int32 {
	recommendedReplicas := int32(currentReplicas)

	if averageUtilization > r.HighWatermark {
		rec := int32(math.Ceil(averageUtilization / r.HighWatermark * currentReplicas))

		if rec > recommendedReplicas {
			recommendedReplicas = rec
		}
	}

	if averageUtilization < r.LowWatermark {
		proposedReplicas := math.Max(math.Floor(averageUtilization/r.LowWatermark*currentReplicas), 1)

		// Adjust to be below the high watermark
		for ; proposedReplicas < currentReplicas; proposedReplicas++ {
			forecastValue := (currentReplicas * averageUtilization / proposedReplicas)

			// Only allow if we don't break the high watermark
			if forecastValue < r.HighWatermark {
				recommendedReplicas = int32(proposedReplicas)
				break
			}
		}
	}

	return recommendedReplicas
}

// Helpers //
func isStaleMetric(currentTime time.Time, metricTimestamp Timestamp) bool {
	return currentTime.Unix()-int64(metricTimestamp) > staleDataThresholdSeconds
}

func average(series []ValueType) float64 {
	average := ValueType(0)
	for _, val := range series {
		average += val
	}
	average /= ValueType(len(series))
	return float64(average)
}

func convertTimestampToTime(timestamp Timestamp) time.Time {
	return time.Unix(int64(timestamp), 0)
}

func convertCpuRequestToMillicores(cpuRequests float64) float64 {
	// Current implementation takes Mi value and returns .AsApproximateFloat64()*100
	// For 100Mi, AsApproximate returns 0.1; we return 10
	// This helper converts value back to Mi units
	return cpuRequests * 10
}
