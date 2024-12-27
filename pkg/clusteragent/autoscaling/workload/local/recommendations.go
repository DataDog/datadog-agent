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
}

type ValueType float64
type Timestamp uint32

// EntityValue represents a value with a timestamp.
type EntityValue struct {
	value     ValueType
	timestamp Timestamp
}

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
	// store = loadstore.NewEntityStore(context.Background()) // TODO: move instantiation
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

func (r resourceRecommenderSettings) recommend(currentTime time.Time, stats QueryResult, currentReplicas float64, pods []*workloadmeta.KubernetesPod) (int32, time.Time, error) {
	podToResources := processPods(pods, r.ContainerName)

	podData := map[string]map[string]EntityValue{}
	for _, result := range stats.results {
		averagedContainers := processAverageContainerMetricValue(result.ContainerValues, currentTime)
		podData[result.PodName] = averagedContainers
	}

	podToUtilization, recommendationTimestamp, missingPods := processPodUtilization(podData, podToResources, r)
	recommendedReplicas := calculateUtilizationRecommendation(podToUtilization, currentReplicas, r)

	// account for missing pods
	if len(missingPods) > 0 {
		adjustedPodToUtilization := adjustMissingPods(getScaleDirection(int32(currentReplicas), recommendedReplicas), podToUtilization, missingPods)
		recommendedReplicas = calculateUtilizationRecommendation(adjustedPodToUtilization, currentReplicas, r)
	}

	return recommendedReplicas, recommendationTimestamp, nil
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

		// stats := store.GetEntitiesStats(dpai.Namespace(), dpai.Spec().TargetRef.Name, nameToMetric[target.ContainerResource.Name])

		rec, ts, err := recSettings.recommend(currentTime, queryResult, float64(*dpai.CurrentReplicas()), pods)
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
func processPods(pods []*workloadmeta.KubernetesPod, containerName *string) map[string]map[string]ContainerRequests {
	podInfo := map[string]map[string]ContainerRequests{}
	for _, pod := range pods {
		podName := pod.Name
		if podInfo[podName] == nil {
			podInfo[podName] = make(map[string]ContainerRequests)
		}
		for _, container := range pod.Containers {
			if containerName != nil && container.Name != *containerName { // ignore all other information if containerName is specified
				continue
			}

			podInfo[podName][container.Name] = ContainerRequests{
				CPU:    container.Resources.CPURequest,
				Memory: container.Resources.MemoryRequest,
			}
		}
	}
	return podInfo
}

// processAverageContainerMetricValue takes a series of metrics and processes them to return the final metric value and
// corresponding timestamp to use to generate a recommendation
// TODO: handle missing containers here?
func processAverageContainerMetricValue(usageTimeseries map[string][]EntityValue, currentTime time.Time) map[string]EntityValue {
	containerToAverageMetric := map[string]EntityValue{}

	for container, series := range usageTimeseries {
		values := []ValueType{}
		for _, entity := range series {
			// Discard stale metrics
			if currentTime.Unix()-int64(entity.timestamp) > staleDataThresholdSeconds {
				continue
			}
			values = append(values, entity.value)
		}

		if len(values) == 0 {
			continue // skip containers with no metrics
		}
		containerToAverageMetric[container] = EntityValue{
			value:     average(values),
			timestamp: series[len(series)-1].timestamp,
		}
	}

	return containerToAverageMetric
}

func calculateUtilizationRecommendation(podToUtilization map[string]float64, currentReplicas float64, recSettings resourceRecommenderSettings) int32 {
	averageUtilization := findAverageUtilization(podToUtilization)
	recommendedReplicas := calculateReplicas(currentReplicas, averageUtilization, recSettings.LowWatermark, recSettings.HighWatermark)

	return recommendedReplicas
}

func processPodUtilization(containerMetrics map[string]map[string]EntityValue, containerRequests map[string]map[string]ContainerRequests, r resourceRecommenderSettings) (map[string]float64, time.Time, []string) {
	podToUtilization := map[string]float64{}
	missingPods := []string{}
	lastTimestamp := time.Time{}
	for pod, podData := range containerRequests {
		metricsData := containerMetrics[pod]
		if metricsData == nil {
			missingPods = append(missingPods, pod)
		}
		utilization, ts := calculatePodUtilization(podData, metricsData, lastTimestamp, r)
		lastTimestamp = ts
		podToUtilization[pod] = utilization
	}

	return podToUtilization, lastTimestamp, missingPods
}

func calculatePodUtilization(podData map[string]ContainerRequests, metricsData map[string]EntityValue, lastTime time.Time, r resourceRecommenderSettings) (float64, time.Time) {
	totalContainerUsage := ValueType(0)
	totalContainerRequests := 0.0

	for container, requests := range podData {
		if containerMetric, ok := metricsData[container]; ok {
			totalContainerUsage += containerMetric.value
			if r.MetricName == "container.memory.usage" {
				totalContainerRequests += float64(*requests.Memory)
			} else {
				totalContainerRequests += *requests.CPU
			}

			// update last timestamp
			metricTime := time.Unix(int64(containerMetric.timestamp), 0)
			if metricTime.After(lastTime) {
				lastTime = metricTime
			}
		}
	}

	return (float64(totalContainerUsage) / totalContainerRequests), lastTime
}

func findAverageUtilization(podToUtilization map[string]float64) float64 {
	totalUtilization := 0.0
	for _, utilization := range podToUtilization {
		totalUtilization += utilization
	}
	return totalUtilization / float64(len(podToUtilization))
}

// Missing pod handling //
func findMissingPods() {
	// loop through and return list of pod ids? that are missing from metrics calculation
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
