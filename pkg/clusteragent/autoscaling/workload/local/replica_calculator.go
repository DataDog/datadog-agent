// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver

// Package local provides local recommendations for autoscaling workloads.
package local

import (
	"errors"
	"fmt"
	"math"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/clock"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/loadstore"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/workload/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	minRequiredMetricDataPoints = 2 // minimum number of data points to consider for a metric

	// Ignore samples from newly-ready pods to avoid scaling on startup bursts.
	readinessWarmupWindow = 30 * time.Second
)

type replicaCalculator struct {
	podWatcher workload.PodWatcher
	clock      clock.Clock
}

type utilizationResult struct {
	// averageUtilization is computed over measured pods only.
	averageUtilization float64
	// measuredPods is the number of pods with usable metrics; it is the average's denominator.
	measuredPods int
	// missingPods is the count of measurable pods without usable metrics; recommend reserves one slot each.
	missingPods             int
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

	targetRef := dpai.Spec().TargetRef
	objectives := dpai.Spec().Objectives
	if dpai.Spec().Fallback != nil && len(dpai.Spec().Fallback.Horizontal.Objectives) > 0 {
		objectives = dpai.Spec().Fallback.Horizontal.Objectives
	}
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
		return nil, fmt.Errorf("No pods found for autoscaler: %s, gvk: %s, name: %s", dpai.ID(), targetGVK.String(), targetRef.Name)
	}

	recommendedReplicas := model.HorizontalScalingValues{}
	var lastUtilizationPct float64

	for _, objective := range objectives {
		recSettings, err := newResourceRecommenderSettings(objective)
		if err != nil {
			return nil, fmt.Errorf("failed to get recommender settings for objective: %s, %s", dpai.ID(), err)
		}
		if recSettings == nil {
			// ControllerObjective is ignored by the local recommender
			continue
		}

		queryResult := lStore.GetMetricsRaw(recSettings.metricName, namespace, podOwnerName, recSettings.containerName)
		rec, utilizationRes, err := recommend(currentTime, *recSettings, pods, queryResult)
		if err != nil {
			log.Debugf("Got error calculating recommendation: %s", err)
			break
		}
		ts := utilizationRes.recommendationTimestamp
		lastUtilizationPct = float64(utilizationRes.averageUtilization)

		if rec > recommendedReplicas.Replicas {
			recommendedReplicas.Replicas = rec
			recommendedReplicas.Timestamp = ts
			recommendedReplicas.Source = datadoghqcommon.DatadogPodAutoscalerLocalValueSource
		}
	}

	if recommendedReplicas.Replicas == 0 {
		return nil, fmt.Errorf("No recommendation found for autoscaler: %s", dpai.ID())
	}

	recommendedReplicas.UtilizationPct = &lastUtilizationPct
	return &recommendedReplicas, nil
}

// recommend computes a replica recommendation for a single objective from observed pod state
// only, so the result is stable and independent of the scaling direction.
//
// Pods are bucketed in calculateUtilization:
//
//   - Measured: Running, Ready, past their warmup window, with usable metrics. They are the
//     sole metric source and denominator.
//   - Missing: Running & Ready but without usable (fresh, post-warmup) metrics. Each is
//     reserved as one replica (neutral fill) — it can neither justify a scale-up (no observed
//     load) nor be scaled away (we keep its capacity until metrics return).
//   - Excluded: terminating/terminal pods (never capacity) and Pending/not-Ready/warming pods
//     (future capacity whose metrics are unreliable). They are left out of both numerator and
//     denominator until they become measurable, which prevents the warmup-burst runaway.
//
// The recommendation is therefore: size for the measured load, then add one slot per missing
// pod.
func recommend(currentTime time.Time, recSettings resourceRecommenderSettings, pods []*workloadmeta.KubernetesPod, queryResult loadstore.QueryResult) (int32, utilizationResult, error) {
	utilizationRes, err := calculateUtilization(recSettings, pods, queryResult, currentTime)
	if err != nil {
		return 0, utilizationResult{}, err
	}

	measuredCount := float64(utilizationRes.measuredPods)
	recommendedReplicas := calculateReplicas(recSettings, measuredCount, utilizationRes.averageUtilization)
	recommendedReplicas += int32(utilizationRes.missingPods)

	return recommendedReplicas, utilizationRes, nil
}

func calculateUtilization(recSettings resourceRecommenderSettings, pods []*workloadmeta.KubernetesPod, queryResult loadstore.QueryResult, currentTime time.Time) (utilizationResult, error) {
	if len(pods) == 0 {
		return utilizationResult{}, errors.New("No pods found")
	}

	if len(queryResult.Results) == 0 {
		return utilizationResult{}, errors.New("Issue fetching metrics data")
	}

	measuredPods := 0
	totalMeasuredUtilization := 0.0
	missingPods := 0
	lastValidTimestamp := time.Time{}

	for _, pod := range pods {
		if !isMeasurablePod(pod, currentTime) {
			continue
		}

		minValidTime := pod.ReadyTimestamp.Add(readinessWarmupWindow)
		utilization, lastTimestamp, ok := calculatePodUtilization(recSettings, pod, queryResult, currentTime, minValidTime)
		if !ok {
			missingPods++
			continue
		}

		measuredPods++
		// Keep the average deterministic by accumulating in pod order.
		totalMeasuredUtilization += utilization
		if lastTimestamp.After(lastValidTimestamp) {
			lastValidTimestamp = lastTimestamp
		}
	}

	if measuredPods == 0 {
		return utilizationResult{}, errors.New("Issue calculating pod utilization")
	}

	return utilizationResult{
		averageUtilization:      totalMeasuredUtilization / float64(measuredPods),
		measuredPods:            measuredPods,
		missingPods:             missingPods,
		recommendationTimestamp: lastValidTimestamp,
	}, nil
}

// isMeasurablePod applies readiness guards used to avoid startup bursts.
func isMeasurablePod(pod *workloadmeta.KubernetesPod, currentTime time.Time) bool {
	if pod.DeletionTimestamp != nil || pod.Phase != string(corev1.PodRunning) {
		return false
	}

	// A nil ReadyTimestamp is treated like HPA's missing ready condition.
	if !pod.Ready || pod.ReadyTimestamp == nil {
		return false
	}

	return !currentTime.Before(pod.ReadyTimestamp.Add(readinessWarmupWindow))
}

// calculatePodUtilization returns ok=false when the pod lacks enough fresh post-warmup data.
func calculatePodUtilization(recSettings resourceRecommenderSettings, pod *workloadmeta.KubernetesPod, queryResult loadstore.QueryResult, currentTime, minValidTime time.Time) (float64, time.Time, bool) {
	totalUsage := 0.0
	totalRequests := 0.0
	latestTimestamp := time.Time{}

	for _, container := range pod.Containers {
		if recSettings.containerName != "" && container.Name != recSettings.containerName {
			continue
		}

		if recSettings.metricName == containerMemoryUsageMetricName && container.Resources.MemoryRequest != nil {
			totalRequests += float64(*container.Resources.MemoryRequest)
		} else if recSettings.metricName == containerCPUUsageMetricName && container.Resources.CPURequest != nil {
			totalRequests += convertCPURequestToNanocores(*container.Resources.CPURequest)
		} else {
			continue
		}

		series := getContainerMetrics(queryResult, pod.Name, container.Name)
		averageValue, lastTimestamp, err := processAverageContainerMetricValue(series, currentTime, minValidTime, recSettings.fallbackStaleDataThreshold)
		if err != nil {
			continue
		}
		totalUsage += averageValue
		if lastTimestamp.After(latestTimestamp) {
			latestTimestamp = lastTimestamp
		}
	}

	if totalRequests > 0 && totalUsage > 0 {
		return totalUsage / totalRequests, latestTimestamp, true
	}
	return 0, time.Time{}, false
}

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

// processAverageContainerMetricValue drops stale and pre-warmup samples before averaging.
// It returns the oldest sample timestamp used in the average.
func processAverageContainerMetricValue(series []loadstore.EntityValue, currentTime time.Time, minValidTime time.Time, fallbackStaleDataThreshold int64) (float64, time.Time, error) {
	values := []loadstore.ValueType{}
	var oldestTimestamp time.Time

	for i := len(series) - 1; i >= 0; i-- {
		entity := series[i]
		if entity.Timestamp == 0 {
			continue
		}

		if isStaleMetric(currentTime, entity.Timestamp, fallbackStaleDataThreshold) {
			continue
		}

		ts := convertTimestampToTime(entity.Timestamp)
		// Keep samples exactly at the warmup boundary, matching HPA's strict-before check.
		// minValidTime is always set: isMeasurablePod guarantees a non-nil ReadyTimestamp.
		if ts.Before(minValidTime) {
			continue
		}

		values = append(values, entity.Value)
		if oldestTimestamp.IsZero() || ts.Before(oldestTimestamp) {
			oldestTimestamp = ts
		}
	}

	if len(values) < minRequiredMetricDataPoints {
		return 0.0, time.Time{}, errors.New("Missing usage metrics")
	}

	return average(values), oldestTimestamp, nil
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
