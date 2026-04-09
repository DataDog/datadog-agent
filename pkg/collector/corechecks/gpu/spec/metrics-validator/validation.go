package main

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
)

const scalarQueryBatchSize = 50

func buildKnownGPUConfigs(architectures *gpuspec.ArchitecturesSpec) []gpuConfig {
	configs := make([]gpuConfig, 0, len(architectures.Architectures)*3)
	for archName, archSpec := range architectures.Architectures {
		modes := []gpuspec.DeviceMode{
			gpuspec.DeviceModePhysical,
			gpuspec.DeviceModeMIG,
			gpuspec.DeviceModeVGPU,
		}
		for _, mode := range modes {
			if !gpuspec.IsModeSupportedByArchitecture(archSpec, mode) {
				continue
			}
			configs = append(configs, gpuConfig{
				Architecture: strings.ToLower(archName),
				DeviceMode:   mode,
				IsKnown:      true,
			})
		}
	}

	slices.SortFunc(configs, func(a, b gpuConfig) int {
		if cmpRes := cmp.Compare(a.Architecture, b.Architecture); cmpRes != 0 {
			return cmpRes
		}
		return cmp.Compare(string(a.DeviceMode), string(b.DeviceMode))
	})
	return configs
}

func getExpectedMetricsForGPUConfig(metricsSpec *gpuspec.MetricsSpec, config gpuConfig) map[string]gpuspec.MetricSpec {
	expected := make(map[string]gpuspec.MetricSpec)
	for metricName, metricSpec := range metricsSpec.Metrics {
		if !metricSpec.SupportsArchitecture(config.Architecture) {
			continue
		}
		if !metricSpec.SupportsDeviceMode(config.DeviceMode) {
			continue
		}
		expected[metricsSpec.MetricPrefix+"."+metricName] = metricSpec
	}
	return expected
}

func getExpectedTagsForMetric(metricsSpec *gpuspec.MetricsSpec, metricSpec gpuspec.MetricSpec) map[string]struct{} {
	tags := map[string]struct{}{}
	for _, tagsetName := range metricSpec.Tagsets {
		tagsetSpec, found := metricsSpec.Tagsets[tagsetName]
		if !found {
			continue
		}
		for _, tag := range tagsetSpec.Tags {
			tags[tag] = struct{}{}
		}
	}
	for _, tag := range metricSpec.CustomTags {
		tags[tag] = struct{}{}
	}
	return tags
}

func buildExpectedTagsByMetric(metricsSpec *gpuspec.MetricsSpec, expectedMetrics map[string]gpuspec.MetricSpec) map[string]map[string]struct{} {
	result := make(map[string]map[string]struct{}, len(expectedMetrics))
	for metricName, metricSpec := range expectedMetrics {
		result[metricName] = getExpectedTagsForMetric(metricsSpec, metricSpec)
	}
	return result
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
	expectedMetricsMap := getExpectedMetricsForGPUConfig(metricsSpec, config)
	expectedMetrics := make([]string, 0, len(expectedMetricsMap))
	for metricName := range expectedMetricsMap {
		expectedMetrics = append(expectedMetrics, metricName)
	}

	deviceCount, err := client.queryDeviceCount(config, fromTS, toTS)
	if err != nil {
		return gpuConfigValidationResult{}, fmt.Errorf("validate gpu config %s/%s: %w", config.Architecture, config.DeviceMode, err)
	}

	result := newGPUConfigValidationResult(config, deviceCount, expectedMetrics)
	if deviceCount == 0 {
		result.State = validationStateMissing
		return result, nil
	}

	expectedTagsByMetric := buildExpectedTagsByMetric(metricsSpec, expectedMetricsMap)
	for _, batch := range batchMetricNames(expectedMetrics, scalarQueryBatchSize) {
		presentMetrics, tagFailures, err := client.queryExpectedMetricsPresenceForGPUConfig(batch, expectedTagsByMetric, config.tagFilter(), fromTS, toTS)
		if err != nil {
			return gpuConfigValidationResult{}, fmt.Errorf("validate expected metrics for %s/%s: %w", config.Architecture, config.DeviceMode, err)
		}
		for metricName := range presentMetrics {
			result.PresentMetrics = append(result.PresentMetrics, metricName)
		}
		for metricName, tags := range tagFailures {
			result.TagFailures[metricName] = append(result.TagFailures[metricName], tags...)
		}
	}

	liveMetrics, err := client.listObservedGPUMetricsForGPUConfig(config, max(toTS-fromTS, int64(0)), metricsSpec.MetricPrefix)
	if err != nil {
		return gpuConfigValidationResult{}, fmt.Errorf("list observed metrics for %s/%s: %w", config.Architecture, config.DeviceMode, err)
	}
	for metricName := range liveMetrics {
		if !slices.Contains(result.ExpectedMetrics, metricName) {
			result.UnknownMetrics = append(result.UnknownMetrics, metricName)
		}
	}

	result.MissingMetrics = computeMissingMetrics(result.ExpectedMetrics, result.PresentMetrics)
	result.State = determineResultState(result)
	return result, nil
}

func combineKnownAndLiveGPUConfigs(knownConfigs []gpuConfig, liveKeys map[string]struct{}) []gpuConfig {
	byKey := make(map[string]gpuConfig, len(knownConfigs)+len(liveKeys))
	for _, config := range knownConfigs {
		byKey[config.key()] = config
	}
	for key := range liveKeys {
		if _, found := byKey[key]; found {
			continue
		}
		parts := strings.Split(key, "|")
		if len(parts) != 2 {
			continue
		}
		byKey[key] = gpuConfig{
			Architecture: parts[0],
			DeviceMode:   gpuspecDeviceMode(parts[1]),
			IsKnown:      false,
		}
	}

	results := make([]gpuConfig, 0, len(byKey))
	for _, config := range byKey {
		results = append(results, config)
	}
	return results
}

func computeValidation(apiKey, appKey, site string, lookbackSeconds int64) (validationResults, error) {
	metricsSpec, err := gpuspec.LoadMetricsSpec()
	if err != nil {
		return validationResults{}, fmt.Errorf("load metrics spec: %w", err)
	}
	architecturesSpec, err := gpuspec.LoadArchitecturesSpec()
	if err != nil {
		return validationResults{}, fmt.Errorf("load architectures spec: %w", err)
	}
	client, err := newMetricsClient(apiKey, appKey, site)
	if err != nil {
		return validationResults{}, fmt.Errorf("create metrics client: %w", err)
	}

	now := time.Now().Unix()
	fromTS := now - lookbackSeconds
	knownConfigs := buildKnownGPUConfigs(architecturesSpec)
	liveConfigKeys, err := client.discoverLiveGPUConfigs(fromTS, now)
	if err != nil {
		return validationResults{}, fmt.Errorf("discover live gpu configs: %w", err)
	}

	configs := combineKnownAndLiveGPUConfigs(knownConfigs, liveConfigKeys)
	results := make([]gpuConfigValidationResult, 0, len(configs))
	failingCount := 0

	for _, config := range configs {
		result, err := validateGPUConfig(client, metricsSpec, config, fromTS, now)
		if err != nil {
			return validationResults{}, fmt.Errorf("validate gpu config %s/%s: %w", config.Architecture, config.DeviceMode, err)
		}
		if !config.IsKnown && result.DeviceCount == 0 {
			continue
		}
		results = append(results, result)
		if config.IsKnown && result.hasFailures() {
			failingCount++
		}
	}

	return validationResults{
		Results:            results,
		MetricsCount:       len(metricsSpec.Metrics),
		ArchitecturesCount: len(architecturesSpec.Architectures),
		FailingCount:       failingCount,
	}, nil
}
