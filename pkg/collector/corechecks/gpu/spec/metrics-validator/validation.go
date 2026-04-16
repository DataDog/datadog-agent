// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main validates emitted GPU metrics against the shared spec.
package main

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

const metricQueryConcurrency = 4

func computeValidation(apiKey, appKey, site string, lookbackSeconds int64) (orgValidationResults, error) {
	specs, err := gpuspec.LoadSpecs()
	if err != nil {
		return orgValidationResults{}, fmt.Errorf("load specs: %w", err)
	}
	client, err := newMetricsClient(apiKey, appKey, site)
	if err != nil {
		return orgValidationResults{}, fmt.Errorf("create metrics client: %w", err)
	}

	now := time.Now().Unix()
	fromTS := now - lookbackSeconds

	configs := gpuspec.KnownGPUConfigs(specs)
	results := make([]gpuConfigValidationResult, 0, len(configs))

	for _, config := range configs {
		result, err := validateGPUConfig(client, specs, config, fromTS, now)
		if err != nil {
			return orgValidationResults{}, fmt.Errorf("validate gpu config %+v: %w", config, err)
		}
		results = append(results, result)
	}

	return orgValidationResults{
		Results:            results,
		MetricsCount:       len(specs.Metrics.Metrics),
		ArchitecturesCount: len(specs.Architectures.Architectures),
	}, nil
}

func validateGPUConfig(client *metricsClient, specs *gpuspec.Specs, config gpuspec.GPUConfig, fromTS, toTS int64) (gpuConfigValidationResult, error) {
	result := gpuConfigValidationResult{
		Config: config,
	}

	expectedMetricsMap := gpuspec.ExpectedMetricsForConfig(specs, config)

	var err error
	result.DeviceCount, err = client.queryDeviceCount(config, fromTS, toTS)
	if err != nil {
		return result, fmt.Errorf("validate gpu config %+v: %w", config, err)
	}

	if result.DeviceCount == 0 {
		result.State = validationStateMissing
		return result, nil
	}

	var mu sync.Mutex
	var group errgroup.Group
	observations := make(map[string][]gpuspec.MetricObservation, len(expectedMetricsMap))
	group.SetLimit(metricQueryConcurrency)

	for metricName, metricSpec := range expectedMetricsMap {
		expectedTags, err := gpuspec.RequiredTagsForMetric(specs.Tags, metricSpec)
		if err != nil {
			return result, fmt.Errorf("derive required tags for %s: %w", metricName, err)
		}

		// Get the metric values
		group.Go(func() error {
			prefixedMetricName := gpuspec.PrefixedMetricName(specs, metricName)
			metricObservations, err := client.queryExpectedMetricPresenceForGPUConfig(prefixedMetricName, expectedTags, config.TagFilter(), fromTS, toTS)
			if err != nil {
				return fmt.Errorf("query expected metric presence for %s: %w", metricName, err)
			}
			for _, observation := range metricObservations {
				observation.Name = metricName
				mu.Lock()
				observations[metricName] = append(observations[metricName], observation)
				mu.Unlock()
			}
			return nil
		})

		tagLookbackSeconds := max(14400, toTS-fromTS) // 4 hours is the minimum lookback for the API

		// Also get tag values for the metric
		group.Go(func() error {
			metricTags, err := client.fetchMetricAllTags(metricName, expectedTags, tagLookbackSeconds, config.TagFilter())
			if err != nil {
				return fmt.Errorf("fetch metric tags for %s: %w", metricName, err)
			}
			mu.Lock()
			observations[metricName] = append(observations[metricName], gpuspec.MetricObservation{
				Name: metricName,
				Tags: metricTags,
			})
			mu.Unlock()
			return nil
		})

		// Finally, get all possible tag values that start with the gpu_ prefix, so that we can check that we aren't missing
		// any tags. This might be redundant with the previous call, but it's better to be redundant than to miss a tag.
		group.Go(func() error {
			wantedTags := map[string]gpuspec.TagSpec{
				"gpu_": {},
			}

			allGpuTags, err := client.fetchMetricAllTags(metricName, wantedTags, tagLookbackSeconds, config.TagFilter())
			if err != nil {
				return fmt.Errorf("fetch metric tags for %s: %w", metricName, err)
			}

			mu.Lock()
			observations[metricName] = append(observations[metricName], gpuspec.MetricObservation{
				Name: metricName,
				Tags: allGpuTags,
			})
			mu.Unlock()
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return result, fmt.Errorf("validate expected metrics for %+v: %w", config, err)
	}

	// Get any other metrics that were emitted with the GPU prefix but aren't in the expected metrics
	liveMetrics, err := client.listObservedGPUMetricsForGPUConfig(config, max(toTS-fromTS, int64(0)), specs.Metrics.MetricPrefix)
	if err != nil {
		return result, fmt.Errorf("list observed metrics for %s/%s: %w", config.Architecture, config.DeviceMode, err)
	}
	for metricName := range liveMetrics {
		if _, found := observations[metricName]; !found {
			// Create an empty slice (no actual values retrieved) but we know it's there, so it will be checked against the spec.
			observations[metricName] = []gpuspec.MetricObservation{}
		}
	}

	result.DetailedResult, err = gpuspec.ValidateEmittedMetricsAgainstSpec(specs, config, observations, nil)
	if err != nil {
		return result, fmt.Errorf("validate observations for %s/%s: %w", config.Architecture, config.DeviceMode, err)
	}

	result.State = determineResultState(result)

	return result, nil
}
