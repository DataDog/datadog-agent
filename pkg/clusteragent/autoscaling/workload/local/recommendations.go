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
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"

	// "github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/loadstore"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type localRecommender struct {
	podWatcher workload.PodWatcher
	// store     *loadstore.EntityStore
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

type ContainerRequests struct {
	CPU    *float64
	Memory *uint64
}

type ContainerInfo struct {
	AveragedMetric          *EntityValue
	Requests                ContainerRequests
	Utilization             float64
	RecommendationTimestamp time.Time
}

type PodEntitiesRequests struct {
	PodName    string
	Containers map[string]ContainerInfo // container name to a time series of entity values, e.g cpu usage from past three collection
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
func (l localRecommender) CalculateHorizontalRecommendations(dpai model.PodAutoscalerInternal) (*model.HorizontalScalingValues, error) {
	currentTime := time.Now()

	// Get current pods for the target
	targetRef := dpai.Spec().TargetRef
	targets := dpai.Spec().Targets
	targetGVK, targetErr := dpai.TargetGVK()
	if targetErr != nil {
		return nil, fmt.Errorf("Failed to get GVK for target: %s, %s", dpai.ID(), targetErr)
	}
	podOwner := workload.NamespacedPodOwner{
		Namespace: dpai.Namespace(),
		Name:      targetRef.Name,
		Kind:      targetGVK.Kind,
	}
	pods := l.podWatcher.GetPodsForOwner(podOwner)
	if len(pods) == 0 {
		// If we found nothing, we'll wait just until the next sync
		return nil, fmt.Errorf("No pods found for autoscaler: %s, gvk: %s, name: %s", dpai.ID(), targetGVK.String(), dpai.Spec().TargetRef.Name)
	}

	recommendedReplicas := model.HorizontalScalingValues{}

	for _, target := range targets {
		recSettings, err := newResourceRecommenderSettings(target)
		if err != nil {
			return nil, fmt.Errorf("Failed to get resource recommender settings: %s", err)
		}

		// queryResult := GetMetricsRaw()
		podData := processPods(pods, recSettings.ContainerName)
		podData = processMetricsData(podData, queryResult, currentTime)

		rec, ts, err := recSettings.recommend(podData, float64(*dpai.CurrentReplicas()))
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

	return &recommendedReplicas, nil
}

// map of pod -> containerName -> requests
func processPods(pods []*workloadmeta.KubernetesPod, containerName *string) map[string]PodEntitiesRequests {
	podData := map[string]PodEntitiesRequests{}

	// Process podwatcher data
	for _, pod := range pods {
		currentPod := PodEntitiesRequests{}
		currentPod.PodName = pod.Name

		for _, container := range pod.Containers {
			// ignore all other information if containerName is specified
			if containerName != nil && container.Name != *containerName {
				continue
			}

			c := ContainerInfo{}
			c.Requests = ContainerRequests{
				CPU:    container.Resources.CPURequest,
				Memory: container.Resources.MemoryRequest,
			}
			currentPod.Containers[container.Name] = c
		}
		podData[pod.Name] = currentPod
	}

	return podData
}

func processMetricsData(podData map[string]PodEntitiesRequests, queryResult QueryResult, currentTime time.Time) map[string]PodEntitiesRequests {
	// Process metrics query
	for _, result := range queryResult.results {
		pod, ok := podData[result.PodName]
		if !ok { // skip; pod does not exist in podWatcher
			continue
		}

		for container, series := range result.ContainerValues {
			containerData, ok := pod.Containers[container]
			if !ok {
				continue
			}
			// calculate average container metric value
			averagedMetric, err := processAverageContainerMetricValue(series, currentTime)
			if err != nil {
				continue
			}
			containerData.AveragedMetric = &averagedMetric
		}
	}
	return podData

}

// processAverageContainerMetricValue takes a series of metrics and processes them to return the final metric value and
// corresponding timestamp to use to generate a recommendation
// TODO: handle missing containers here?
func processAverageContainerMetricValue(series []EntityValue, currentTime time.Time) (EntityValue, error) {
	values := []ValueType{}
	lastTimestamp := Timestamp(0)

	for _, entity := range series {
		// Discard stale metrics
		if currentTime.Unix()-int64(entity.timestamp) > staleDataThresholdSeconds {
			continue
		}
		values = append(values, entity.value)
		lastTimestamp = entity.timestamp
	}

	if len(values) == 0 {
		return EntityValue{}, fmt.Errorf("Missing usage metrics")
	}

	return EntityValue{
		value:     average(values),
		timestamp: lastTimestamp,
	}, nil
}

func (r resourceRecommenderSettings) recommend(podData map[string]PodEntitiesRequests, currentReplicas float64) (int32, time.Time, error) {
	podToUtilization, recommendationTimestamp, missingPods := processPodUtilization(podData, r)
	recommendedReplicas := calculateUtilizationRecommendation(podToUtilization, currentReplicas, r)

	// account for missing pods
	if len(missingPods) > 0 {
		adjustedPodToUtilization := adjustMissingPods(getScaleDirection(int32(currentReplicas), recommendedReplicas), podToUtilization, missingPods)
		recommendedReplicas = calculateUtilizationRecommendation(adjustedPodToUtilization, currentReplicas, r)
	}

	return recommendedReplicas, recommendationTimestamp, nil
}

func calculateUtilizationRecommendation(podToUtilization map[string]float64, currentReplicas float64, recSettings resourceRecommenderSettings) int32 {
	averageUtilization := findAverageUtilization(podToUtilization)
	recommendedReplicas := calculateReplicas(currentReplicas, averageUtilization, recSettings.LowWatermark, recSettings.HighWatermark)

	return recommendedReplicas
}

func processPodUtilization(podData map[string]PodEntitiesRequests, r resourceRecommenderSettings) (map[string]float64, time.Time, []string) {
	podToUtilization := map[string]float64{}
	missingPods := []string{}
	lastTimestamp := time.Time{}

	for pod, data := range podData {
		isPodProcessed := false // track if metrics were reported for the pod
		for _, containerData := range data.Containers {
			utilization, ts, err := calculatePodUtilization(containerData, lastTimestamp, r)
			if err != nil {
				continue
			}

			lastTimestamp = ts
			podToUtilization[pod] = utilization
			isPodProcessed = true
		}
		if !isPodProcessed {
			missingPods = append(missingPods, pod)
		}
	}

	return podToUtilization, lastTimestamp, missingPods

}

// calculatePodUtilization calculates the utilization of a pod based on data from separate containers
func calculatePodUtilization(containerData ContainerInfo, lastTimestamp time.Time, r resourceRecommenderSettings) (float64, time.Time, error) {
	if containerData.AveragedMetric == nil {
		return 0, lastTimestamp, fmt.Errorf("Missing usage metrics")
	}

	totalContainerUsage := containerData.AveragedMetric.value
	totalContainerRequests := 0.0

	if r.MetricName == "container.memory.usage" {
		totalContainerRequests += float64(*containerData.Requests.Memory)
	} else {
		totalContainerRequests += *containerData.Requests.CPU
	}

	// update last timestamp
	metricTime := time.Unix(int64(containerData.AveragedMetric.timestamp), 0)
	if metricTime.After(lastTimestamp) {
		lastTimestamp = metricTime
	}

	return (float64(totalContainerUsage) / totalContainerRequests), lastTimestamp, nil
}

func findAverageUtilization(podToUtilization map[string]float64) float64 {
	totalUtilization := 0.0
	for _, utilization := range podToUtilization {
		totalUtilization += utilization
	}
	return totalUtilization / float64(len(podToUtilization))
}

func adjustMissingPods(scaleDirection workload.ScaleDirection, podToUtilization map[string]float64, missingPods []string) map[string]float64 {
	for _, pod := range missingPods {
		// adjust based on scale direction
		if scaleDirection == workload.ScaleUp {
			podToUtilization[pod] = 0.0 // 0%
		} else if scaleDirection == workload.ScaleDown {
			podToUtilization[pod] = 1.0 // 100%
		}
	}
	return podToUtilization
}

func calculateReplicas(currentReplicas float64, averageUtilization float64, lowWatermark float64, highWatermark float64) int32 {
	recommendedReplicas := int32(0)

	if averageUtilization > highWatermark {
		rec := int32(math.Ceil(averageUtilization / highWatermark * currentReplicas))

		if rec > recommendedReplicas {
			recommendedReplicas = rec
		}
	}

	if averageUtilization < lowWatermark {
		proposedReplicas := math.Max(math.Floor(averageUtilization/lowWatermark*currentReplicas), 1)

		// Adjust to be below the high watermark
		for ; proposedReplicas < currentReplicas; proposedReplicas++ {
			forecastValue := (currentReplicas * averageUtilization / proposedReplicas)

			// Only allow if we don't break the high watermark
			if forecastValue < highWatermark {
				rec := int32(proposedReplicas)

				if rec > recommendedReplicas {
					recommendedReplicas = rec
				}
				break
			}
		}
	}

	return recommendedReplicas
}

// Helpers //
func average(series []ValueType) ValueType {
	average := ValueType(0)
	for _, val := range series {
		average += val
	}
	average /= ValueType(len(series))
	return average
}

// TODO move this to general util
func getScaleDirection(currentReplicas, recommendedReplicas int32) workload.ScaleDirection {
	if currentReplicas < recommendedReplicas {
		return workload.ScaleUp
	} else if currentReplicas > recommendedReplicas {
		return workload.ScaleDown
	}
	return workload.NoScale
}
