// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build kubeapiserver

package local

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghq "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/loadstore"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
)

type localRecommender struct {
	podWatcher workload.PodWatcher
}

type Timeseries struct {
	Epochs []uint64
	Values []float64
}

const (
	staleDataThresholdSeconds = 180 // 3 minutes
)

var (
	podResourceToMetric = map[corev1.ResourceName]string{
		corev1.ResourceCPU: "kubernetes.pod.cpu.usage.req_pct.dist",
	}
	containerResourceToMetric = map[corev1.ResourceName]string{
		corev1.ResourceCPU: "container.cpu.usage.rec_pct.dist",
	}
	store = loadstore.NewEntityStore(context.Background()) // TODO: move instantiation
)

type resourceRecommenderSettings struct {
	MetricName    string
	ContainerName *string
	LowWatermark  float64
	HighWatermark float64
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
	if target.Value.Type != v1alpha1.DatadogPodAutoscalerUtilizationTargetValueType {
		return nil, fmt.Errorf("invalid value type: %s", target.Value.Type)
	}
	if target.Name != v1.ResourceCPU {
		return nil, fmt.Errorf("invalid resource name: %s", target.Name)
	}

	recSettings := &resourceRecommenderSettings{
		MetricName:    podResourceToMetric[target.Name],
		LowWatermark:  float64((*target.Value.Utilization - 5)) / 100.0,
		HighWatermark: float64((*target.Value.Utilization + 5)) / 100.0,
	}
	return recSettings, nil
}

func getOptionsFromContainerResource(target *datadoghq.DatadogPodAutoscalerContainerResourceTarget) (*resourceRecommenderSettings, error) {
	if target == nil {
		return nil, fmt.Errorf("nil target")
	}
	if target.Value.Type != v1alpha1.DatadogPodAutoscalerUtilizationTargetValueType {
		return nil, fmt.Errorf("invalid value type: %s", target.Value.Type)
	}
	if target.Name != v1.ResourceCPU {
		return nil, fmt.Errorf("invalid resource name: %s", target.Name)
	}

	recSettings := &resourceRecommenderSettings{
		MetricName:    containerResourceToMetric[target.Name],
		LowWatermark:  float64((*target.Value.Utilization - 5)) / 100.0,
		HighWatermark: float64((*target.Value.Utilization + 5)) / 100.0,
	}
	return recSettings, nil
}

func (r resourceRecommenderSettings) recommend(currentTime time.Time, dpai model.PodAutoscalerInternal) (int32, time.Time, error) {
	recommendedReplicas := int32(0)
	recommendationTimestamp := time.Time{}
	// stats := store.GetEntitiesStats(dpai.Namespace(), dpai.Spec().TargetRef.Name, nameToMetric[target.ContainerResource.Name])
	stats := Timeseries{
		Epochs: []uint64{1, 2, 3},
		Values: []float64{1.0, 2.0, 3.0},
	}

	ts, metricValue, err := processMetricsValues(stats)
	if err != nil {
		return recommendedReplicas, recommendationTimestamp, fmt.Errorf("Failed to process metrics values: %s", err)
	}

	// Discard stale metrics
	if currentTime.Unix()-int64(ts) > staleDataThresholdSeconds {
		return recommendedReplicas, recommendationTimestamp, fmt.Errorf("Metrics are stale")
	}

	// TODO: account for missing pods
	// missingPods := []string{}
	// check if each pod returned from podwatcher has a metric - if not then adjust based on scale direction (hpa)
	// OR
	// query for count metric -> get number of pods this way (backend recommender)

	currentReplicas := float64(*dpai.CurrentReplicas())
	if metricValue > r.HighWatermark {
		rec := int32(math.Ceil(metricValue / r.HighWatermark * currentReplicas))

		if rec > recommendedReplicas {
			recommendedReplicas = rec
			recommendationTimestamp = time.Unix(int64(ts), 0)
		}
	}
	if metricValue < r.LowWatermark {
		proposedReplicas := math.Max(math.Floor(metricValue/r.LowWatermark*currentReplicas), 1)

		// Adjust to be below the high watermark
		for ; proposedReplicas < currentReplicas; proposedReplicas++ {
			forecastValue := (currentReplicas * metricValue / proposedReplicas)

			// Only allow if we don't break the high watermark
			if forecastValue < r.HighWatermark {
				rec := int32(proposedReplicas)

				if rec > recommendedReplicas {
					recommendedReplicas = rec
					recommendationTimestamp = time.Unix(int64(ts), 0)
				}
				break
			}
		}
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
		rec, ts, err := recSettings.recommend(currentTime, dpai)
		if err != nil {
			log.Debugf("Got error calculating recommendation: %s", err)
			break
		}

		// always choose the highest recommendation given
		if rec > recommendedReplicas.Replicas {
			recommendedReplicas.Replicas = rec
			recommendedReplicas.Timestamp = ts
			// recommendedReplicas.Source = local
		}
	}

	// TODO: need error handling in caller to check err and not to update (we don't want to recommend 0)
	if recommendedReplicas.Replicas == 0 {
		return nil, fmt.Errorf("No recommendation found for autoscaler: %s", dpai.ID())
	}

	return &recommendedReplicas, nil
}

// processMetricsValues takes a series of metrics and processes them to return the final metric value and
// corresponding timestamp to use to generate a recommendation
func processMetricsValues(usageTimeseries Timeseries) (uint64, float64, error) {
	// usageMetrics should have a length of 3; we want to do the rollup and ignore N-1

	var timestamp uint64
	var value float64

	if len(usageTimeseries.Epochs) != len(usageTimeseries.Values) || len(usageTimeseries.Epochs) == 0 {
		return timestamp, value, fmt.Errorf("Missing usage metrics")
	}

	if len(usageTimeseries.Epochs) < 3 { // average all data points
		for _, val := range usageTimeseries.Values {
			value += val
		}
		value /= float64(len(usageTimeseries.Values))
		timestamp = usageTimeseries.Epochs[len(usageTimeseries.Epochs)-1]
	} else { // rollup: construct a complete 30s interval with data - discard most recent point
		resIndex := len(usageTimeseries.Epochs) - 1
		value = average([]float64{usageTimeseries.Values[resIndex-1], usageTimeseries.Values[resIndex-2]})
		timestamp = usageTimeseries.Epochs[resIndex-1]
	}

	return timestamp, value, nil
}

// Missing pod handling //
func findMissingPods() {
	// loop through and return list of pod ids? that are missing from metrics calculation
}

func adjustMissingPods(scaleDirection workload.ScaleDirection, metrics map[string]float64, podIds []string) map[string]float64 {
	for _, pod := range podIds {
		if _, ok := metrics[pod]; !ok {
			// adjust based on scale direction
			if scaleDirection == workload.ScaleUp {
				metrics[pod] = 0.0 // 0%
			} else if scaleDirection == workload.ScaleDown {
				metrics[pod] = 1.0 // 100%
			}
		}
	}
	return metrics
}

// Helpers //
func average(series []float64) float64 {
	average := 0.0
	for _, val := range series {
		average += val
	}
	average /= float64(len(series))
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
