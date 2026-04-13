// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package main validates emitted GPU metrics against the shared spec.
package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

const metricQueryConcurrency = 4

func computeValidation(apiKey, appKey, site string, lookbackSeconds int64) (orgValidationResults, error) {
	metricsSpec, err := gpuspec.LoadMetricsSpec()
	if err != nil {
		return orgValidationResults{}, fmt.Errorf("load metrics spec: %w", err)
	}
	tagsSpec, err := gpuspec.LoadTagsSpec()
	if err != nil {
		return orgValidationResults{}, fmt.Errorf("load tags spec: %w", err)
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
		result, err := validateGPUConfig(client, metricsSpec, tagsSpec, config, fromTS, now)
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

func validateGPUConfig(client *metricsClient, metricsSpec *gpuspec.MetricsSpec, tagsSpec *gpuspec.TagsSpec, config gpuspec.GPUConfig, fromTS, toTS int64) (gpuConfigValidationResult, error) {
	result := gpuConfigValidationResult{
		Config: config,
	}

	expectedMetricsMap := gpuspec.ExpectedMetricsForConfig(metricsSpec, config)

	deviceCount, err := client.queryDeviceCount(config, fromTS, toTS)
	if err != nil {
		return result, fmt.Errorf("validate gpu config %s/%s: %w", config.Architecture, config.DeviceMode, err)
	}
	result.DeviceCount = deviceCount

	if result.DeviceCount == 0 {
		result.State = validationStateMissing
		return result, nil
	}

	expectedTagsByMetric, err := gpuspec.RequiredTagsByMetric(tagsSpec, expectedMetricsMap)
	if err != nil {
		return result, fmt.Errorf("derive required tags for %s/%s: %w", config.Architecture, config.DeviceMode, err)
	}

	observations := make(map[string][]gpuspec.MetricObservation, len(expectedMetricsMap))

	var mu sync.Mutex
	var group errgroup.Group
	group.SetLimit(metricQueryConcurrency)

	for metricName := range expectedMetricsMap {
		group.Go(func() error {
			prefixedMetricName := gpuspec.PrefixedMetricName(metricsSpec, metricName)
			metricObservations, err := client.queryExpectedMetricPresenceForGPUConfig(prefixedMetricName, expectedTagsByMetric[metricName], config.TagFilter(), fromTS, toTS)
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
	}

	if err := group.Wait(); err != nil {
		return result, fmt.Errorf("validate expected metrics for %+v: %w", config, err)
	}

	liveMetrics, err := client.listObservedGPUMetricsForGPUConfig(config, max(toTS-fromTS, int64(0)), metricsSpec.MetricPrefix)
	if err != nil {
		return result, fmt.Errorf("list observed metrics for %s/%s: %w", config.Architecture, config.DeviceMode, err)
	}
	for metricName := range liveMetrics {
		if _, found := observations[metricName]; !found {
			observations[metricName] = []gpuspec.MetricObservation{}
		}
	}

	result.DetailedResult, err = gpuspec.ValidateEmittedMetricsAgainstSpec(metricsSpec, tagsSpec, config, observations, nil)
	if err != nil {
		return result, fmt.Errorf("validate observations for %s/%s: %w", config.Architecture, config.DeviceMode, err)
	}

	result.State = determineResultState(result)
	return result, nil
}

func collectRegexValidatedTags(expectedTags map[string]gpuspec.TagSpec, tagNameFilter string) map[string]*regexp.Regexp {
	result := make(map[string]*regexp.Regexp)
	for tagName, tagSpec := range expectedTags {
		if tagSpec.Regex == nil {
			continue
		}
		if tagNameFilter != "" && !strings.Contains(tagName, tagNameFilter) {
			continue
		}
		result[tagName] = tagSpec.Regex
	}
	return result
}

func computeTagValidation(apiKey, appKey, site, metricNameFilter, tagNameFilter string, windowSeconds int64, metricScopeFilter string) (tagValidationResults, error) {
	metricsSpec, err := gpuspec.LoadMetricsSpec()
	if err != nil {
		return tagValidationResults{}, fmt.Errorf("load metrics spec: %w", err)
	}
	tagsSpec, err := gpuspec.LoadTagsSpec()
	if err != nil {
		return tagValidationResults{}, fmt.Errorf("load tags spec: %w", err)
	}
	client, err := newMetricsClient(apiKey, appKey, site)
	if err != nil {
		return tagValidationResults{}, fmt.Errorf("create metrics client: %w", err)
	}

	failures := map[string]map[string][]string{}
	errors := []string{}

	for relativeMetricName, metricSpec := range metricsSpec.Metrics {
		metricName := metricsSpec.MetricPrefix + "." + relativeMetricName
		if metricNameFilter != "" && !strings.Contains(metricName, metricNameFilter) {
			continue
		}

		expectedTags, err := gpuspec.RequiredTagsForMetric(tagsSpec, metricSpec)
		if err != nil {
			errors = append(errors, fmt.Sprintf("derive required tags for %s: %v", metricName, err))
			continue
		}
		regexTags := collectRegexValidatedTags(expectedTags, tagNameFilter)
		if len(regexTags) == 0 {
			continue
		}

		allTags, err := client.fetchMetricAllTags(metricName, regexTags, windowSeconds, metricScopeFilter)
		if err != nil {
			errors = append(errors, fmt.Sprintf("validate tags for %s: %v", metricName, err))
			continue
		}

		invalidValues := map[string][]string{}
		for tagName, compiledRegex := range regexTags {
			values := allTags[tagName]
			mismatches := make([]string, 0, len(values))
			for _, value := range values {
				if !compiledRegex.MatchString(value) {
					mismatches = append(mismatches, value)
				}
			}
			if len(mismatches) == 0 {
				continue
			}
			slices.Sort(mismatches)
			mismatches = slices.Compact(mismatches)
			invalidValues[tagName] = mismatches
		}
		if len(invalidValues) > 0 {
			failures[metricName] = invalidValues
		}
	}

	return tagValidationResults{
		Site:     site,
		Failures: failures,
		Errors:   errors,
	}, nil
}
