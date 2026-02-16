// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package validation

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/nvidia"
	ddnvml "github.com/DataDog/datadog-agent/pkg/gpu/safenvml"
	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

// specFile is the YAML metric specification
type specFile struct {
	Namespace  string          `yaml:"namespace"`
	Collectors []specCollector `yaml:"collectors"`
	Deprecated []struct {
		Name       string `yaml:"name"`
		ReplacedBy string `yaml:"replaced_by"`
	} `yaml:"deprecated_metrics"`
}

type specCollector struct {
	Name     string       `yaml:"name"`
	CodeName string       `yaml:"code_name"`
	Metrics  []specMetric `yaml:"metrics"`
}

type specMetric struct {
	Name       string `yaml:"name"`
	Type       string `yaml:"type"`
	Priority   string `yaml:"priority"`
	PerProcess bool   `yaml:"per_process"`
	DedupGroup string `yaml:"dedup_group"`
	CustomTags []string `yaml:"custom_tags"`
}

// collectedMetric represents a metric collected from a specific collector
type collectedMetric struct {
	name       string
	priority   nvidia.MetricPriority
	collector  string
	hasWorkloads bool
	hasTags   bool
}

func loadSpec(t *testing.T) *specFile {
	t.Helper()
	data, err := os.ReadFile("spec/gpu_metrics.yaml")
	require.NoError(t, err, "failed to read spec file")

	var spec specFile
	require.NoError(t, yaml.Unmarshal(data, &spec))
	return &spec
}

// priorityFromString converts a priority string to MetricPriority
func priorityFromString(s string) nvidia.MetricPriority {
	switch s {
	case "high":
		return nvidia.High
	case "medium":
		return nvidia.Medium
	case "medium_low":
		return nvidia.MediumLow
	case "low":
		return nvidia.Low
	default:
		return nvidia.Low
	}
}

// buildCollectorsForTest creates all collectors using mock infrastructure and collects metrics
func buildCollectorsForTest(t *testing.T) map[string][]collectedMetric {
	t.Helper()

	// Set up NVML mock with all functions mocked
	nvmlMock := testutil.GetBasicNvmlMockWithOptions(
		testutil.WithMIGDisabled(),
		testutil.WithMockAllFunctions(),
		testutil.WithDeviceCount(1), // Single device for simpler validation
	)
	ddnvml.WithMockNVML(t, nvmlMock)
	deviceCache := ddnvml.NewDeviceCache()

	// Start events gatherer
	eventsGatherer := nvidia.NewDeviceEventsGatherer()
	require.NoError(t, eventsGatherer.Start())
	t.Cleanup(func() { require.NoError(t, eventsGatherer.Stop()) })

	// Set up system-probe cache with test data
	spCache := &nvidia.SystemProbeCache{}
	spCache.SetStatsForTest(&model.GPUStats{
		ProcessMetrics: []model.ProcessStatsTuple{
			{
				Key: model.ProcessStatsKey{
					PID:        1234,
					DeviceUUID: testutil.DefaultGpuUUID,
				},
				UtilizationMetrics: model.UtilizationMetrics{
					UsedCores:     0.5,
					ActiveTimePct: 42.0,
					Memory: model.MemoryMetrics{
						CurrentBytes: 1024,
					},
				},
			},
		},
		DeviceMetrics: []model.DeviceStatsTuple{
			{
				DeviceUUID: testutil.DefaultGpuUUID,
				Metrics: model.UtilizationMetrics{
					ActiveTimePct: 50.0,
				},
			},
		},
	})

	devices, err := deviceCache.AllPhysicalDevices()
	require.NoError(t, err)

	deps := &nvidia.CollectorDependencies{
		DeviceEventsGatherer: eventsGatherer,
		SystemProbeCache:     spCache,
		Workloadmeta:         testutil.GetWorkloadMetaMockWithDefaultGPUs(t),
	}

	collectors, err := nvidia.BuildCollectors(devices, deps, nil)
	require.NoError(t, err)

	// Collect metrics from each collector
	result := make(map[string][]collectedMetric)
	for _, c := range collectors {
		name := string(c.Name())
		metrics, err := c.Collect()
		// Allow errors (some collectors may have partial results)
		if err != nil {
			t.Logf("Collector %s returned error (may be expected): %v", name, err)
		}

		for _, m := range metrics {
			result[name] = append(result[name], collectedMetric{
				name:         m.Name,
				priority:     m.Priority,
				collector:    name,
				hasWorkloads: len(m.AssociatedWorkloads) > 0,
				hasTags:      len(m.Tags) > 0,
			})
		}
	}

	return result
}

// TestSpecMetricsMatchCode validates that every metric in the YAML spec is emitted by the code
func TestSpecMetricsMatchCode(t *testing.T) {
	spec := loadSpec(t)
	collectedByCollector := buildCollectorsForTest(t)

	// Build a set of all collected metric names per collector
	type metricKey struct {
		collector string
		name      string
	}
	collectedSet := make(map[metricKey][]collectedMetric)
	for collectorName, metrics := range collectedByCollector {
		for _, m := range metrics {
			key := metricKey{collector: collectorName, name: m.name}
			collectedSet[key] = append(collectedSet[key], m)
		}
	}

	// For each metric in spec, check it was emitted
	for _, specCollector := range spec.Collectors {
		collectorName := specCollector.CodeName

		// Get the unique metric names for this collector in the spec
		// (some metrics appear twice with different priorities, e.g. process.memory.usage)
		specMetricNames := make(map[string][]specMetric)
		for _, m := range specCollector.Metrics {
			specMetricNames[m.Name] = append(specMetricNames[m.Name], m)
		}

		for metricName, specMetrics := range specMetricNames {
			key := metricKey{collector: collectorName, name: metricName}
			collected := collectedSet[key]

			// device_events collector won't emit metrics without actual XID events
			if collectorName == "deviceEvents" {
				continue
			}

			// Fields collector metrics that require rate computation won't emit on first collection
			if collectorName == "fields" {
				// NVLink throughput metrics compute rates and need 2 collection cycles
				isRateMetric := false
				for _, sm := range specMetrics {
					if sm.Name == "nvlink.throughput.data.rx" ||
						sm.Name == "nvlink.throughput.data.tx" ||
						sm.Name == "nvlink.throughput.raw.rx" ||
						sm.Name == "nvlink.throughput.raw.tx" {
						isRateMetric = true
						break
					}
				}
				if isRateMetric {
					continue
				}
			}

			assert.NotEmpty(t, collected,
				"spec metric %q from collector %q was not emitted by code",
				metricName, collectorName)
		}
	}
}

// TestCodeMetricsMatchSpec validates that every metric emitted by code exists in the spec
func TestCodeMetricsMatchSpec(t *testing.T) {
	spec := loadSpec(t)
	collectedByCollector := buildCollectorsForTest(t)

	// Build lookup: collector code_name -> set of metric names in spec
	specMetricsByCollector := make(map[string]map[string]bool)
	for _, specCollector := range spec.Collectors {
		names := make(map[string]bool)
		for _, m := range specCollector.Metrics {
			names[m.Name] = true
		}
		specMetricsByCollector[specCollector.CodeName] = names
	}

	// Check every collected metric exists in spec
	for collectorName, metrics := range collectedByCollector {
		specMetrics, collectorInSpec := specMetricsByCollector[collectorName]
		assert.True(t, collectorInSpec,
			"collector %q emits metrics but is not in the spec", collectorName)

		if !collectorInSpec {
			continue
		}

		for _, m := range metrics {
			assert.True(t, specMetrics[m.name],
				"metric %q emitted by collector %q is not in the spec (undocumented metric?)",
				m.name, collectorName)
		}
	}
}

// TestMetricPriorities validates that priority values in the spec match what the code emits
func TestMetricPriorities(t *testing.T) {
	spec := loadSpec(t)
	collectedByCollector := buildCollectorsForTest(t)

	for _, specCollector := range spec.Collectors {
		collectorName := specCollector.CodeName
		collected := collectedByCollector[collectorName]
		if len(collected) == 0 {
			continue
		}

		// Build map of metric name -> collected priorities
		collectedPriorities := make(map[string]map[nvidia.MetricPriority]bool)
		for _, m := range collected {
			if collectedPriorities[m.name] == nil {
				collectedPriorities[m.name] = make(map[nvidia.MetricPriority]bool)
			}
			collectedPriorities[m.name][m.priority] = true
		}

		for _, specMetric := range specCollector.Metrics {
			priorities, found := collectedPriorities[specMetric.Name]
			if !found {
				continue // Already checked in TestSpecMetricsMatchCode
			}

			expectedPriority := priorityFromString(specMetric.Priority)
			assert.True(t, priorities[expectedPriority],
				"metric %q from collector %q: spec says priority %q (%d) but code emits priorities %v",
				specMetric.Name, collectorName, specMetric.Priority, expectedPriority, priorities)
		}
	}
}

// TestPerProcessMetrics validates that per-process metrics have workload associations
func TestPerProcessMetrics(t *testing.T) {
	spec := loadSpec(t)
	collectedByCollector := buildCollectorsForTest(t)

	for _, specCollector := range spec.Collectors {
		collectorName := specCollector.CodeName
		collected := collectedByCollector[collectorName]
		if len(collected) == 0 {
			continue
		}

		// Build per-process metrics set from spec
		perProcessMetrics := make(map[string]bool)
		for _, m := range specCollector.Metrics {
			if m.PerProcess {
				perProcessMetrics[m.Name] = true
			}
		}

		for _, m := range collected {
			if perProcessMetrics[m.name] {
				assert.True(t, m.hasWorkloads,
					"metric %q from collector %q is marked per_process in spec but has no AssociatedWorkloads",
					m.name, collectorName)
			}
		}
	}
}

// TestCustomTagMetrics validates that metrics with custom_tags in spec have Tags set
func TestCustomTagMetrics(t *testing.T) {
	spec := loadSpec(t)
	collectedByCollector := buildCollectorsForTest(t)

	for _, specCollector := range spec.Collectors {
		collectorName := specCollector.CodeName
		collected := collectedByCollector[collectorName]
		if len(collected) == 0 {
			continue
		}

		// Build custom tag metrics set from spec
		customTagMetrics := make(map[string]bool)
		for _, m := range specCollector.Metrics {
			if len(m.CustomTags) > 0 {
				customTagMetrics[m.Name] = true
			}
		}

		for _, m := range collected {
			if customTagMetrics[m.name] {
				assert.True(t, m.hasTags,
					"metric %q from collector %q has custom_tags in spec but no Tags in collected metric",
					m.name, collectorName)
			}
		}
	}
}

// TestSpecCompleteness performs a summary check of the spec
func TestSpecCompleteness(t *testing.T) {
	spec := loadSpec(t)

	totalMetrics := 0
	for _, c := range spec.Collectors {
		totalMetrics += len(c.Metrics)
	}

	t.Logf("Spec summary:")
	t.Logf("  Namespace: %s", spec.Namespace)
	t.Logf("  Collectors: %d", len(spec.Collectors))
	t.Logf("  Total metric entries: %d", totalMetrics)
	t.Logf("  Deprecated metrics: %d", len(spec.Deprecated))

	for _, c := range spec.Collectors {
		uniqueNames := make(map[string]bool)
		for _, m := range c.Metrics {
			uniqueNames[m.Name] = true
		}
		t.Logf("  Collector %q (%s): %d entries, %d unique metric names",
			c.Name, c.CodeName, len(c.Metrics), len(uniqueNames))
	}

	// Sanity checks
	assert.Equal(t, "gpu", spec.Namespace)
	assert.GreaterOrEqual(t, len(spec.Collectors), 5, "expected at least 5 collectors")
	assert.GreaterOrEqual(t, totalMetrics, 50, "expected at least 50 metric entries")
}

// TestDedupGroups validates dedup group consistency
func TestDedupGroups(t *testing.T) {
	spec := loadSpec(t)

	// Collect all dedup groups across collectors
	type dedupEntry struct {
		collector string
		metric    specMetric
	}
	dedupGroups := make(map[string][]dedupEntry)

	for _, c := range spec.Collectors {
		for _, m := range c.Metrics {
			if m.DedupGroup != "" {
				dedupGroups[m.DedupGroup] = append(dedupGroups[m.DedupGroup], dedupEntry{
					collector: c.CodeName,
					metric:    m,
				})
			}
		}
	}

	for groupName, entries := range dedupGroups {
		// All entries in a dedup group should have the same metric name
		firstName := entries[0].metric.Name
		for _, e := range entries[1:] {
			assert.Equal(t, firstName, e.metric.Name,
				"dedup group %q has inconsistent metric names: %q vs %q",
				groupName, firstName, e.metric.Name)
		}

		// There should be at least 2 entries (otherwise dedup is pointless)
		assert.GreaterOrEqual(t, len(entries), 2,
			"dedup group %q has only %d entry, expected at least 2", groupName, len(entries))

		// Log the dedup group for visibility
		priorities := make([]string, len(entries))
		for i, e := range entries {
			priorities[i] = fmt.Sprintf("%s(%s)", e.collector, e.metric.Priority)
		}
		t.Logf("Dedup group %q (%s): %v", groupName, firstName, priorities)
	}
}
