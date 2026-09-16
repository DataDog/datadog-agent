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

func computeValidation(apiKey, appKey, site string, lookbackSeconds int64, agentVersion, metricFilter string) (orgValidationResults, error) {
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

	tagInventoryExtraFilters := []string{metricFilter}
	if strings.TrimSpace(agentVersion) != "" {
		versionFilter, err := client.filterForAgentVersion(agentVersion, fromTS, now)
		if err != nil {
			return orgValidationResults{}, fmt.Errorf("build filter for agent version %q: %w", agentVersion, err)
		}
		log.Printf("targeting agent version %q", agentVersion)
		log.Printf("using version-derived cluster metric filter %q", versionFilter.metricFilter)
		log.Printf("using %d version-derived tag inventory filter(s): %q", len(versionFilter.tagFilters), versionFilter.tagFilters)
		metricFilter = combineMetricFilters(versionFilter.metricFilter, metricFilter)
		tagInventoryExtraFilters = versionFilter.tagFilters
	}

	configs := gpuspec.KnownGPUConfigs(specs)
	results := make([]gpuConfigValidationResult, 0, len(configs))

	var allErrors error
	for _, config := range configs {
		nvLinkCapability := "n/a"
		if config.NVLinkCapable != nil {
			nvLinkCapability = fmt.Sprintf("%t", *config.NVLinkCapable)
		}
		log.Printf("validating gpu config %s/%s (NVLink capable: %s)", config.Architecture, config.DeviceMode, nvLinkCapability)
		result, err := validateGPUConfig(client, specs, config, metricFilter, tagInventoryExtraFilters, fromTS, now)
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

func validateGPUConfig(client *metricsClient, specs *gpuspec.Specs, config gpuspec.GPUConfig, metricFilter string, tagInventoryExtraFilters []string, fromTS, toTS int64) (gpuConfigValidationResult, error) {
	result := gpuConfigValidationResult{
		Config: config,
		State:  validationStateMissing,
	}

	validationOptions := gpuspec.ValidationOptions{
		WorkloadActive:  true,
		ConfigFeatures:  gpuspec.AllConfigFeatures(),
		WorkloadTagsets: gpuspec.AllWorkloadTagsets(specs.Tags),
	}
	expectedMetricsMap := gpuspec.ExpectedMetricsForConfig(specs, config, validationOptions)
	queryFilter := combineMetricFilters(config.TagFilter(), metricFilter)
	tagInventoryFilters := tagInventoryFiltersForConfig(config, tagInventoryExtraFilters)

	var err error
	result.DeviceCount, err = client.queryDeviceCount(config, queryFilter, fromTS, toTS)
	if err != nil {
		result.RetrievalErrors = append(result.RetrievalErrors, fmt.Sprintf("query device count: %v", err))
		result.State = determineResultState(result)
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
	unavailableMetrics := make(map[string]bool)
	group.SetLimit(metricQueryConcurrency)

	for metricName, metricSpec := range expectedMetricsMap {
		prefixedMetricName := gpuspec.PrefixedMetricName(specs, metricName)
		validatesValues := metricSpec.Validator.HasStaticValueValidation()
		expectedTags, err := gpuspec.ExpectedTagsForMetricWithOptions(specs.Tags, metricSpec, validationOptions)
		if err != nil {
			return result, fmt.Errorf("derive expected tags for %s: %w", metricName, err)
		}

		// Get the metric values
		group.Go(func() error {
			metricObservations, err := client.queryExpectedMetricPresenceForGPUConfig(prefixedMetricName, expectedTags, queryFilter, fromTS, toTS, validatesValues)
			if err != nil {
				retrievalError := fmt.Errorf("query expected metric presence for %s: %w", metricName, err)
				mu.Lock()
				unavailableMetrics[metricName] = true
				result.RetrievalErrors = append(result.RetrievalErrors, retrievalError.Error())
				mu.Unlock()
				return retrievalError
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

		tagInventoryPrefixes := tagInventoryPrefixesForMetric(expectedTags)

		// Also get tag values for the metric. Physical GPU configs use multiple positive
		// all-tags scopes because the endpoint does not handle NOT filters like scalar queries do.
		for _, tagInventoryFilter := range tagInventoryFilters {
			group.Go(func() error {
				metricTags, err := client.fetchMetricAllTags(prefixedMetricName, tagInventoryPrefixes, tagLookbackSeconds, tagInventoryFilter)
				if err != nil {
					retrievalError := fmt.Errorf("fetch metric tags for %s: %w", metricName, err)
					mu.Lock()
					result.RetrievalErrors = append(result.RetrievalErrors, retrievalError.Error())
					mu.Unlock()
					return retrievalError
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
		retrievalError := fmt.Errorf("list observed gpu metrics: %w", err)
		result.RetrievalErrors = append(result.RetrievalErrors, retrievalError.Error())
		allErrors = errors.Join(allErrors, retrievalError)
	}

	for metricName := range liveMetrics {
		if _, found := observations[metricName]; !found {
			// Create an empty slice (no actual values retrieved) but we know it's there, so it will be checked against the spec.
			observations[metricName] = []gpuspec.MetricObservation{}
		}
	}

	validationOptions.IgnoreMetrics = unavailableMetrics
	result.DetailedResult, err = gpuspec.ValidateEmittedMetricsAgainstSpec(specs, config, observations, nil, validationOptions)
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

func tagInventoryFiltersForConfig(config gpuspec.GPUConfig, extraFilters []string) []string {
	// The metric all-tags endpoint accepts a comma-separated list of positive tag
	// filters. Use equivalent positive scopes for physical GPUs, then query each
	// selected cluster separately because repeated tag keys are ANDed, not ORed.
	baseParts := []string{"gpu_architecture:" + config.Architecture}
	hasClusterFilter := slices.ContainsFunc(extraFilters, func(filter string) bool {
		return strings.HasPrefix(strings.TrimSpace(filter), "kube_cluster_name:")
	})
	if !hasClusterFilter {
		baseParts = append(baseParts, "kube_cluster_name:*")
	}
	if config.NVLinkCapable != nil {
		baseParts = append(baseParts, fmt.Sprintf("gpu_nvlink_capable:%t", *config.NVLinkCapable))
	}
	var configFilters []string
	switch config.DeviceMode {
	case gpuspec.DeviceModeMIG:
		configFilters = []string{strings.Join(append(baseParts, "gpu_slicing_mode:mig"), ",")}
	case gpuspec.DeviceModeVGPU:
		configFilters = []string{strings.Join(append(baseParts, "gpu_virtualization_mode:*vgpu"), ",")}
	default:
		configFilters = []string{
			strings.Join(append(slices.Clone(baseParts), "gpu_slicing_mode:none", "gpu_virtualization_mode:none"), ","),
			strings.Join(append(slices.Clone(baseParts), "gpu_slicing_mode:none", "gpu_virtualization_mode:passthrough"), ","),
		}
	}

	if len(extraFilters) == 0 {
		return configFilters
	}
	filters := make([]string, 0, len(configFilters)*len(extraFilters))
	for _, configFilter := range configFilters {
		for _, extraFilter := range extraFilters {
			if strings.TrimSpace(extraFilter) == "" {
				filters = append(filters, configFilter)
				continue
			}
			filters = append(filters, strings.Join([]string{configFilter, extraFilter}, ","))
		}
	}
	return filters
}

func tagInventoryPrefixesForMetric(expectedTags map[string]gpuspec.TagSpec) map[string]gpuspec.TagSpec {
	prefixes := maps.Clone(expectedTags)
	prefixes["gpu_"] = gpuspec.TagSpec{}
	return prefixes
}
