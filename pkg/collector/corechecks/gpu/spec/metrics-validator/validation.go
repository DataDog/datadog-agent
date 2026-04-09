package main

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

const scalarQueryBatchSize = 50

func buildKnownGPUConfigs(architectures *gpuspec.ArchitecturesSpec) []gpuConfig {
	specConfigs := gpuspec.KnownGPUConfigs(architectures)
	configs := make([]gpuConfig, 0, len(specConfigs))
	for _, config := range specConfigs {
		configs = append(configs, gpuConfig{
			Architecture: config.Architecture,
			DeviceMode:   config.DeviceMode,
			IsKnown:      true,
		})
	}
	return configs
}

func batchMetricNames(metricNames []string, chunkSize int) [][]string {
	batches := make([][]string, 0, (len(metricNames)+chunkSize-1)/chunkSize)
	for start := 0; start < len(metricNames); start += chunkSize {
		end := min(start+chunkSize, len(metricNames))
		batches = append(batches, metricNames[start:end])
	}
	return batches
}

func validateGPUConfig(client *metricsClient, metricsSpec *gpuspec.MetricsSpec, config gpuConfig, fromTS, toTS int64) (gpuConfigValidationResult, error) {
	result := gpuConfigValidationResult{
		Config: config,
	}

	expectedMetricsMap := gpuspec.ExpectedMetricsForConfig(metricsSpec, config.Architecture, config.DeviceMode)
	result.ExpectedMetrics = len(expectedMetricsMap)

	var err error
	result.DeviceCount, err = client.queryDeviceCount(config, fromTS, toTS)
	if err != nil {
		return result, fmt.Errorf("validate gpu config %s/%s: %w", config.Architecture, config.DeviceMode, err)
	}

	if result.DeviceCount == 0 {
		result.State = validationStateMissing
		return result, nil
	}

	expectedTagsByMetric, err := gpuspec.RequiredTagsByMetric(metricsSpec, expectedMetricsMap)
	if err != nil {
		return result, fmt.Errorf("derive required tags for %s/%s: %w", config.Architecture, config.DeviceMode, err)
	}

	observations := make(map[string][]gpuspec.MetricObservation, len(expectedMetricsMap))
	for _, batch := range batchMetricNames(maps.Keys(expectedMetricsMap), scalarQueryBatchSize) {
		batchObservations, err := client.queryExpectedMetricsPresenceForGPUConfig(batch, expectedTagsByMetric, config.tagFilter(), fromTS, toTS)
		if err != nil {
			return result, fmt.Errorf("validate expected metrics for %s/%s: %w", config.Architecture, config.DeviceMode, err)
		}
		for metricName, observation := range batchObservations {
			observations[metricName] = append(observations[metricName], observation)
		}
	}

	liveMetrics, err := client.listObservedGPUMetricsForGPUConfig(config, max(toTS-fromTS, int64(0)), metricsSpec.MetricPrefix)
	if err != nil {
		return result, fmt.Errorf("list observed metrics for %s/%s: %w", config.Architecture, config.DeviceMode, err)
	}
	for metricName := range liveMetrics {
		if _, found := observations[metricName]; !found {
			// Create an empty slice (no actual values retrieved) but we know it's there, so it will be checked against the spec
			observations[metricName] = []gpuspec.MetricObservation{}
		}
	}

	result.PresentMetrics = len(observations)

	result.DetailedResult, err = gpuspec.ValidateEmittedMetricsAgainstSpec(metricsSpec, config.Architecture, config.DeviceMode, observations, nil)
	if err != nil {
		return result, fmt.Errorf("validate observations for %s/%s: %w", config.Architecture, config.DeviceMode, err)
	}

	for _, status := range result.DetailedResult.Metrics {
		for _, error := range status.Errors {
			if errors.Is(error, gpuspec.ErrorMissing) {
				result.MissingMetrics++
			} else if errors.Is(error, gpuspec.ErrorUnknown) {
				result.UnknownMetrics++
			}

			if len(status.TagErrors) > 0 {
				result.TagFailures++
			}
		}
	}

	result.State = determineResultState(result)

	return result, nil
}

func combineKnownAndLiveGPUConfigs(knownConfigs []gpuConfig, liveConfigs []gpuConfig) []gpuConfig {
	for _, liveCfg := range liveConfigs {
		isAlreadyKnown := slices.ContainsFunc(knownConfigs, func(cfg gpuConfig) bool {
			return cfg.equals(liveCfg)
		})
		if !isAlreadyKnown {
			knownConfigs = append(knownConfigs, liveCfg)
		}
	}
	return knownConfigs
}

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
	knownConfigs := buildKnownGPUConfigs(architecturesSpec)
	liveConfigs, err := client.discoverLiveGPUConfigs(fromTS, now)
	if err != nil {
		return orgValidationResults{}, fmt.Errorf("discover live gpu configs: %w", err)
	}

	configs := combineKnownAndLiveGPUConfigs(knownConfigs, liveConfigs)
	results := make([]gpuConfigValidationResult, 0, len(configs))
	failingCount := 0

	for _, config := range configs {
		result, err := validateGPUConfig(client, metricsSpec, config, fromTS, now)
		if err != nil {
			return orgValidationResults{}, fmt.Errorf("validate gpu config %s/%s: %w", config.Architecture, config.DeviceMode, err)
		}
		if !config.IsKnown && result.DeviceCount == 0 {
			continue
		}
		results = append(results, result)
		if config.IsKnown && result.hasFailures() {
			failingCount++
		}
	}

	return orgValidationResults{
		Results:            results,
		MetricsCount:       len(metricsSpec.Metrics),
		ArchitecturesCount: len(architecturesSpec.Architectures),
		FailingCount:       failingCount,
	}, nil
}
