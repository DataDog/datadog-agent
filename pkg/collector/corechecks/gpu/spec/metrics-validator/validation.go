// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main validates emitted GPU metrics against the shared spec.
package main

import (
	"errors"
	"fmt"
	"log"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

const metricQueryConcurrency = 4

func computeValidation(apiKey, appKey, site string, lookbackSeconds int64, metricFilter string) (orgValidationResults, error) {
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

	var allErrors error
	for _, config := range configs {
		log.Printf("validating gpu config %s/%s", config.Architecture, config.DeviceMode)
		result, err := validateGPUConfig(client, specs, config, metricFilter, fromTS, now)
		if err != nil {
			allErrors = errors.Join(allErrors, fmt.Errorf("validate gpu config %+v: %w", config, err))
		}
		results = append(results, result)
	}

	return orgValidationResults{
		Results:            results,
		MetricsCount:       len(specs.Metrics.Metrics),
		ArchitecturesCount: len(specs.Architectures.Architectures),
	}, allErrors
}

func validateGPUConfig(client *metricsClient, specs *gpuspec.Specs, config gpuspec.GPUConfig, metricFilter string, fromTS, toTS int64) (gpuConfigValidationResult, error) {
	result := gpuConfigValidationResult{
		Config: config,
		State:  validationStateMissing,
	}

	// We still query all tags and metrics, even if they're workload only and some live hosts won't have them.
	// The relaxation happens in the render phase, where metrics/tags that are sometimes missing will not be reported
	// as errors if they're workload-only.
	expectedMetricsMap := gpuspec.ExpectedMetricsForConfig(specs, config, gpuspec.ValidationOptions{
		WorkloadActive: true,
	})
	queryFilter := combineMetricFilters(config.TagFilter(), metricFilter)
	tagInventoryFilters := tagInventoryFiltersForConfig(config, metricFilter)

	var err error
	result.DeviceCount, err = client.queryDeviceCount(config, queryFilter, fromTS, toTS)
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
	tagObservations := make(map[string][]gpuspec.MetricObservation, len(expectedMetricsMap))
	group.SetLimit(metricQueryConcurrency)

	for metricName, metricSpec := range expectedMetricsMap {
		prefixedMetricName := gpuspec.PrefixedMetricName(specs, metricName)
		validatesValues := metricSpec.Validator != nil
		requiredTags, workloadOnlyTags, err := gpuspec.RequiredTagsForMetric(specs.Tags, metricSpec)
		if err != nil {
			return result, fmt.Errorf("derive required tags for %s: %w", metricName, err)
		}

		maps.Copy(requiredTags, workloadOnlyTags) // include workload tags as required for the tag validation

		// Get the metric values
		group.Go(func() error {
			metricObservations, err := client.queryExpectedMetricPresenceForGPUConfig(prefixedMetricName, requiredTags, queryFilter, fromTS, toTS, validatesValues)
			if err != nil {
				return fmt.Errorf("query expected metric presence for %s: %w", metricName, err)
			}

			if len(metricObservations) == 0 {
				return nil
			}

			mu.Lock()
			observations[metricName] = append(observations[metricName], metricObservations...)
			mu.Unlock()

			return nil
		})

		tagLookbackSeconds := max(14400, toTS-fromTS) // 4 hours is the minimum lookback for the API

		tagInventoryPrefixes := tagInventoryPrefixesForMetric(requiredTags)

		// Also get tag values for the metric. Physical GPU configs use multiple positive
		// all-tags scopes because the endpoint does not handle NOT filters like scalar queries do.
		for _, tagInventoryFilter := range tagInventoryFilters {
			group.Go(func() error {
				metricTags, err := client.fetchMetricAllTags(prefixedMetricName, tagInventoryPrefixes, tagLookbackSeconds, tagInventoryFilter)
				if err != nil {
					return fmt.Errorf("fetch metric tags for %s: %w", metricName, err)
				}
				if len(metricTags) == 0 {
					return nil
				}

				mu.Lock()
				tagObservations[metricName] = append(tagObservations[metricName], gpuspec.MetricObservation{
					Name: metricName,
					Tags: metricTags,
				})
				mu.Unlock()
				return nil
			})
		}

	}

	// Do not return early on errors, just try doing everything we can
	var allErrors error
	if err := group.Wait(); err != nil {
		allErrors = errors.Join(allErrors, fmt.Errorf("error retrieving observations: %w", err))
	}

	for metricName, obs := range observations {
		// Only add tag observations if  the metric was found for this specific config.
		// the metric APIs will return empty tag lists for metrics that are emitted in the org but not with the given GPU config.
		// If we added those observations, we would have false negatives for missing tags.
		if len(obs) == 0 {
			continue
		}
		observations[metricName] = append(observations[metricName], tagObservations[metricName]...)
	}

	// Get any other metrics that were emitted with the GPU prefix but aren't in the expected metrics
	liveMetrics, err := client.listObservedGPUMetricsForGPUConfig(config, queryFilter, max(toTS-fromTS, int64(0)), specs.Metrics.MetricPrefix)
	if err != nil {
		allErrors = errors.Join(allErrors, fmt.Errorf("error listing observed gpu metrics: %w", err))
	}

	for metricName := range liveMetrics {
		if _, found := observations[metricName]; !found {
			// Create an empty slice (no actual values retrieved) but we know it's there, so it will be checked against the spec.
			observations[metricName] = []gpuspec.MetricObservation{}
		}
	}

	result.DetailedResult, err = gpuspec.ValidateEmittedMetricsAgainstSpec(specs, config, observations, nil, gpuspec.ValidationOptions{
		WorkloadActive: true,
	})
	if err != nil {
		allErrors = errors.Join(allErrors, fmt.Errorf("error validating emitted metrics against spec: %w", err))
	}

	result.State = determineResultState(result)

	return result, allErrors
}

func combineMetricFilters(filters ...string) string {
	parts := make([]string, 0, len(filters))
	for _, filter := range filters {
		filter = strings.TrimSpace(filter)
		if filter == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("(%s)", filter))
	}
	return strings.Join(parts, " AND ")
}

func tagInventoryFiltersForConfig(config gpuspec.GPUConfig, extraFilter string) []string {
	// The metric all-tags endpoint does not handle NOT filters like scalar metric queries do.
	// Use equivalent positive scopes for physical GPUs so tag inventories stay complete.
	baseParts := []string{"kube_cluster_name:*", "gpu_architecture:" + config.Architecture}
	switch config.DeviceMode {
	case gpuspec.DeviceModeMIG:
		baseParts = append(baseParts, "gpu_slicing_mode:mig")
	case gpuspec.DeviceModeVGPU:
		baseParts = append(baseParts, "gpu_virtualization_mode:*vgpu")
	default:
		filters := []string{
			strings.Join(append(slices.Clone(baseParts), "gpu_slicing_mode:none", "gpu_virtualization_mode:none"), " AND "),
			strings.Join(append(slices.Clone(baseParts), "gpu_slicing_mode:none", "gpu_virtualization_mode:passthrough"), " AND "),
		}
		if strings.TrimSpace(extraFilter) == "" {
			return filters
		}
		return []string{
			combineMetricFilters(filters[0], extraFilter),
			combineMetricFilters(filters[1], extraFilter),
		}
	}

	return []string{combineMetricFilters(strings.Join(baseParts, " AND "), extraFilter)}
}

func tagInventoryPrefixesForMetric(requiredTags map[string]gpuspec.TagSpec) map[string]gpuspec.TagSpec {
	prefixes := maps.Clone(requiredTags)
	prefixes["gpu_"] = gpuspec.TagSpec{}
	return prefixes
}
