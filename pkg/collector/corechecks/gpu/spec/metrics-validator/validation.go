// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main validates emitted GPU metrics against the shared spec.
package main

import (
	"fmt"
	"log"
	"time"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

func computeValidation(apiKey, appKey, site string, lookbackSeconds int64) (orgValidationResults, error) {
	metricsSpec, err := gpuspec.LoadMetricsSpec()
	if err != nil {
		return orgValidationResults{}, fmt.Errorf("load metrics spec: %w", err)
	}
	architecturesSpec, err := gpuspec.LoadArchitecturesSpec()
	if err != nil {
		return orgValidationResults{}, fmt.Errorf("load architectures spec: %w", err)
	}
	client, err := newMetricsClient(apiKey, appKey, site)
	if err != nil {
		return orgValidationResults{}, fmt.Errorf("create metrics client: %w", err)
	}

	now := time.Now().Unix()
	fromTS := now - lookbackSeconds

	configs := gpuspec.KnownGPUConfigs(architecturesSpec)
	results := make([]gpuConfigValidationResult, 0, len(configs))

	for _, config := range configs {
		log.Printf("validating gpu config %s/%s", config.Architecture, config.DeviceMode)
		result, err := validateGPUConfig(client, metricsSpec, config, fromTS, now)
		if err != nil {
			return orgValidationResults{}, fmt.Errorf("validate gpu config %s/%s: %w", config.Architecture, config.DeviceMode, err)
		}

		results = append(results, result)
	}

	return orgValidationResults{
		Results:            results,
		MetricsCount:       len(metricsSpec.Metrics),
		ArchitecturesCount: len(architecturesSpec.Architectures),
	}, nil
}

func validateGPUConfig(client *metricsClient, metricsSpec *gpuspec.MetricsSpec, config gpuspec.GPUConfig, fromTS, toTS int64) (gpuConfigValidationResult, error) {
	result := gpuConfigValidationResult{
		Config: config,
	}

	expectedMetricsMap := gpuspec.ExpectedMetricsForConfig(metricsSpec, config)

	var err error
	result.DeviceCount, err = client.queryDeviceCount(config, fromTS, toTS)
	if err != nil {
		return result, fmt.Errorf("validate gpu config %+v: %w", config, err)
	}

	if result.DeviceCount == 0 {
		result.State = validationStateMissing
		return result, nil
	}

	expectedTagsByMetric, err := gpuspec.RequiredTagsByMetric(metricsSpec, expectedMetricsMap)
	if err != nil {
		return result, fmt.Errorf("derive required tags for %+v: %w", config, err)
	}

	observations := make(map[string][]gpuspec.MetricObservation, len(expectedMetricsMap))
	expectedMetricNames := make([]string, 0, len(expectedMetricsMap))
	for metricName := range expectedMetricsMap {
		expectedMetricNames = append(expectedMetricNames, metricName)
	}

	for _, metricName := range expectedMetricNames {
		prefixedMetricName := gpuspec.PrefixedMetricName(metricsSpec, metricName)
		metricObservations, err := client.queryExpectedMetricPresenceForGPUConfig(prefixedMetricName, expectedTagsByMetric[metricName], config.TagFilter(), fromTS, toTS)
		if err != nil {
			return result, fmt.Errorf("validate expected metrics for %+v: %w", config, err)
		}
		for _, observation := range metricObservations {
			observation.Name = metricName
			observations[metricName] = append(observations[metricName], observation)
		}
	}

	// Get any other metrics that were emitted with the GPU prefix but aren't in the expected metrics.
	liveMetrics, err := client.listObservedGPUMetricsForGPUConfig(config, max(toTS-fromTS, int64(0)), metricsSpec.MetricPrefix)
	if err != nil {
		return result, fmt.Errorf("list observed metrics for %+v: %w", config, err)
	}
	for metricName := range liveMetrics {
		if _, found := observations[metricName]; !found {
			// Create an empty slice (no actual values retrieved) but we know it's there, so it will be checked against the spec
			observations[metricName] = []gpuspec.MetricObservation{}
		}
	}

	result.DetailedResult, err = gpuspec.ValidateEmittedMetricsAgainstSpec(metricsSpec, config, observations, nil)
	if err != nil {
		return result, fmt.Errorf("validate observations for %+v: %w", config, err)
	}

	result.State = determineResultState(result)

	return result, nil
}
