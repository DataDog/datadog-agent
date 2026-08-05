// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sync"
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func requireCounterMetricValueBySource(t *testing.T, source string, want float64, telemetryComp telemetry.Component) {
	t.Helper()

	metricFamilies, err := telemetryComp.Gather(false)
	require.NoError(t, err)

	metricName := "observer__" + telemetryFilteredMetrics
	for _, family := range metricFamilies {
		if family.GetName() != metricName {
			continue
		}

		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["source"] == source {
				assert.Equal(t, want, metric.GetCounter().GetValue())
				return
			}
		}
	}

	t.Fatalf("counter %q with source=%q not found", metricName, source)
}

func requireCounterMetricValueForNameBySource(t *testing.T, metricName, source string, want float64, telemetryComp telemetry.Component) {
	t.Helper()

	metricFamilies, err := telemetryComp.Gather(false)
	require.NoError(t, err)

	fullMetricName := "observer__" + metricName
	for _, family := range metricFamilies {
		if family.GetName() != fullMetricName {
			continue
		}

		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["source"] == source {
				assert.Equal(t, want, metric.GetCounter().GetValue())
				return
			}
		}
	}

	t.Fatalf("counter %q with source=%q not found", fullMetricName, source)
}

func requireNoCounterMetricForNameBySource(t *testing.T, metricName, source string, telemetryComp telemetry.Component) {
	t.Helper()

	metricFamilies, err := telemetryComp.Gather(false)
	require.NoError(t, err)

	fullMetricName := "observer__" + metricName
	for _, family := range metricFamilies {
		if family.GetName() != fullMetricName {
			continue
		}

		for _, metric := range family.GetMetric() {
			labels := map[string]string{}
			for _, label := range metric.GetLabel() {
				labels[label.GetName()] = label.GetValue()
			}
			if labels["source"] == source {
				t.Fatalf("unexpected counter %q with source=%q found", fullMetricName, source)
			}
		}
	}
}

func TestMetricsFilterRulesMuteSetBlocksMatchingMetric(t *testing.T) {
	filter, err := newDefaultMetricsFilterRules()
	require.NoError(t, err)

	tags := []string{"env:prod"}
	h := seriesKeyHash("check", "system.cpu.user", tags)
	filter.setMuted(map[uint64]struct{}{h: {}})

	assert.False(t, filter.isAllowed("system.cpu.user", "check", tags))
	assert.True(t, filter.isAllowed("system.mem.used", "check", tags))
	assert.True(t, filter.isAllowed("system.cpu.user", "dogstatsd", tags))
	// LogMetricsExtractorName bypasses the mute check entirely
	assert.True(t, filter.isAllowed("system.cpu.user", LogMetricsExtractorName, tags))
}

func TestMetricsFilterRulesAllowWithoutRules(t *testing.T) {
	filter, err := newMetricsFilterRules(nil)
	require.NoError(t, err)

	assert.True(t, filter.isAllowed("system.cpu.user", "dogstatsd", []string{"env:dev"}))
	assert.True(t, filter.isAllowed("kubernetes.cpu.usage", "check", nil))
}

func TestMetricsFilterRulesExcludeAtMatchByNamePattern(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type:        excludeAtMatch,
		Name:        "drop_kubernetes",
		NamePattern: "kubernetes.*",
	}})
	require.NoError(t, err)

	assert.False(t, filter.isAllowed("kubernetes.cpu.usage", "check", nil))
	assert.True(t, filter.isAllowed("system.cpu.user", "check", nil))
}

func TestMetricsFilterRulesIncludeAtMatchShortCircuitsButFallthroughAllows(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type:        includeAtMatch,
		Name:        "keep_kubernetes",
		NamePattern: "kubernetes.*",
	}})
	require.NoError(t, err)

	assert.True(t, filter.isAllowed("kubernetes.cpu.usage", "check", nil))
	assert.True(t, filter.isAllowed("system.cpu.user", "check", nil))
}

func TestMetricsFilterRulesTagAndSemantics(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type: excludeAtMatch,
		Name: "drop_dev_web",
		Tags: []string{"env:dev", "service:web"},
	}})
	require.NoError(t, err)

	assert.False(t, filter.isAllowed("system.cpu.user", "dogstatsd", []string{"env:dev", "service:web"}))
	assert.True(t, filter.isAllowed("system.cpu.user", "dogstatsd", []string{"env:dev"}))
	assert.True(t, filter.isAllowed("system.cpu.user", "dogstatsd", []string{"service:web"}))
}

func TestMetricsFilterRulesSourceFilter(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type:   excludeAtMatch,
		Name:   "drop_dogstatsd_dev",
		Source: "dogstatsd",
		Tags:   []string{"env:dev"},
	}})
	require.NoError(t, err)

	assert.False(t, filter.isAllowed("system.cpu.user", "dogstatsd", []string{"env:dev"}))
	assert.True(t, filter.isAllowed("system.cpu.user", "check", []string{"env:dev"}))
}

func TestMetricsFilterRulesOrderedEvaluationFirstMatchWins(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{
		{
			Type:        excludeAtMatch,
			Name:        "drop_kubernetes",
			NamePattern: "kubernetes.*",
		},
		{
			Type:        includeAtMatch,
			Name:        "keep_prod_kubernetes",
			NamePattern: "kubernetes.*",
			Tags:        []string{"env:prod"},
		},
	})
	require.NoError(t, err)

	assert.False(t, filter.isAllowed("kubernetes.cpu.usage", "check", []string{"env:prod"}))
}

func TestMetricsFilterRulesLogMetricsExtractorBypass(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type: excludeAtMatch,
		Name: "drop_everything",
	}})
	require.NoError(t, err)

	assert.True(t, filter.isAllowed("log.pattern.error.count", LogMetricsExtractorName, []string{"service:web"}))
	assert.False(t, filter.isAllowed("system.cpu.user", "dogstatsd", nil))
}

func TestImplicitMetricsProcessingRulesExcludeObserverTelemetry(t *testing.T) {
	filter, err := newMetricsFilterRules(implicitMetricsProcessingRules())
	require.NoError(t, err)

	assert.False(t, filter.isAllowed(observerTelemetryMetricPrefix+"metrics.filtered", observerdef.AgentNamespace, nil))
	assert.True(t, filter.isAllowed(observerTelemetryMetricPrefix+"metrics.filtered", "dogstatsd", nil))
	assert.True(t, filter.isAllowed("datadog.agent.running", observerdef.AgentNamespace, nil))
	assert.True(t, filter.isAllowed("system.cpu.user", "dogstatsd", nil))
}

func TestImplicitObserverTelemetryRuleCannotBeOverridden(t *testing.T) {
	cfg := configmock.NewFromYAML(t, `
anomaly_detection:
  metrics:
    processing_rules:
      - type: include_at_match
        name: keep_observer_telemetry
        name_pattern: datadog.agent.observer.*
`)
	filter, err := loadMetricFilter(cfg)
	require.NoError(t, err)

	assert.False(t, filter.isAllowed(observerTelemetryMetricPrefix+"metrics.filtered", observerdef.AgentNamespace, nil))
}

func TestLoadMetricFilterWithoutConfigUsesDefaultRules(t *testing.T) {
	filter, err := loadMetricFilter(nil)
	require.NoError(t, err)
	require.NotNil(t, filter)

	assert.False(t, filter.isAllowed(observerTelemetryMetricPrefix+"metrics.filtered", observerdef.AgentNamespace, nil))
	assert.True(t, filter.isAllowed("datadog.agent.running", observerdef.AgentNamespace, nil))
	assert.True(t, filter.isAllowed("system.cpu.user", "dogstatsd", nil))
}

func TestMetricsFilterRulesValidationErrors(t *testing.T) {
	_, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type: excludeAtMatch,
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name is required")

	_, err = newMetricsFilterRules([]metricsProcessingRule{{
		Type: "drop",
		Name: "bad_type",
	}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported type")
}

func TestMetricsFilterRulesEmptyNamePatternMatchesAllNames(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type: excludeAtMatch,
		Name: "drop_prod_everywhere",
		Tags: []string{"env:prod"},
	}})
	require.NoError(t, err)

	assert.False(t, filter.isAllowed("system.cpu.user", "dogstatsd", []string{"env:prod"}))
	assert.False(t, filter.isAllowed("kubernetes.cpu.usage", "check", []string{"env:prod"}))
}

func TestMetricsFilterRulesExactPrefixNamePatternWithoutWildcard(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type:        excludeAtMatch,
		Name:        "drop_flaky_counter",
		NamePattern: "flaky.counter",
	}})
	require.NoError(t, err)

	assert.False(t, filter.isAllowed("flaky.counter", "dogstatsd", nil))
	assert.False(t, filter.isAllowed("flaky.counter.rate", "dogstatsd", nil))
	assert.True(t, filter.isAllowed("flaky.count", "dogstatsd", nil))
}

func TestPrepareMetricIngestDropsMatchingMetrics(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type:   excludeAtMatch,
		Name:   "drop_dev_dogstatsd",
		Source: "dogstatsd",
		Tags:   []string{"env:dev"},
	}})
	require.NoError(t, err)

	dropped := prepareMetricIngest("dogstatsd", &metricObs{name: "system.cpu.user", tags: []string{"env:dev"}}, filter)
	assert.Nil(t, dropped.metric)

	kept := prepareMetricIngest("dogstatsd", &metricObs{name: "system.cpu.user", value: 1, tags: []string{"env:prod"}}, filter)
	require.NotNil(t, kept.metric)
	assert.Equal(t, "dogstatsd", kept.source)
	assert.Equal(t, "system.cpu.user", kept.metric.name)
}

func TestPrepareMetricIngestAllowsInternalAgentMetricsAndDropsObserverTelemetry(t *testing.T) {
	filter, err := newDefaultMetricsFilterRules()
	require.NoError(t, err)
	allowed := prepareMetricIngest("dogstatsd", &metricObs{
		name:  "datadog.agent.running",
		value: 1,
	}, filter)
	require.NotNil(t, allowed.metric)
	assert.Equal(t, observerdef.AgentNamespace, allowed.source)

	dropped := prepareMetricIngest("dogstatsd", &metricObs{
		name:  observerTelemetryMetricPrefix + "metrics.filtered",
		value: 1,
	}, filter)
	assert.Nil(t, dropped.metric)
}

func TestPrepareMetricIngestAllowsNormalizedAgentMetricsWhenIncludedEarlier(t *testing.T) {
	filter, err := newMetricsFilterRules(append([]metricsProcessingRule{
		{
			Type:   includeAtMatch,
			Name:   "keep_agent_metrics",
			Source: observerdef.AgentNamespace,
		},
	}, implicitMetricsProcessingRules()...))
	require.NoError(t, err)

	decision := prepareMetricIngest("dogstatsd", &metricObs{
		name:      "datadog.agent.running",
		value:     1,
		timestamp: 1000,
	}, filter)
	require.NotNil(t, decision.metric)
	assert.Equal(t, observerdef.AgentNamespace, decision.source)
}

func TestPrepareMetricIngestMixedAgentRulesKeepIncludedMetricAndDropOthers(t *testing.T) {
	filter, err := newMetricsFilterRules(append([]metricsProcessingRule{
		{
			Type:        includeAtMatch,
			Name:        "keep_agent_running",
			NamePattern: "datadog.agent.running",
			Source:      observerdef.AgentNamespace,
		},
		{
			Type:   excludeAtMatch,
			Name:   "drop_other_agent_metrics",
			Source: observerdef.AgentNamespace,
		},
	}, implicitMetricsProcessingRules()...))
	require.NoError(t, err)

	kept := prepareMetricIngest("dogstatsd", &metricObs{
		name:      "datadog.agent.running",
		value:     1,
		timestamp: 1000,
	}, filter)
	require.NotNil(t, kept.metric)
	assert.Equal(t, observerdef.AgentNamespace, kept.source)

	dropped := prepareMetricIngest("dogstatsd", &metricObs{
		name:      "datadog.agent.uptime",
		value:     1,
		timestamp: 1000,
	}, filter)
	assert.Nil(t, dropped.metric)
	assert.Equal(t, observerdef.AgentNamespace, dropped.source)
}

func TestObserverAppliesMetricFilterBySource(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type:   excludeAtMatch,
		Name:   "drop_dogstatsd",
		Source: "dogstatsd",
	}})
	require.NoError(t, err)

	storage := newTimeSeriesStorage()
	eng := newEngine(engineConfig{storage: storage})
	obs := &observerImpl{
		engine:               eng,
		obsCh:                make(chan observation, 16),
		ingestMetricsEnabled: true,
		metricFilter:         filter,
	}
	obs.handleFunc = obs.innerHandle

	var (
		wg        sync.WaitGroup
		closeOnce sync.Once
	)
	stopFn := func() {
		closeOnce.Do(func() { close(obs.obsCh) })
		wg.Wait()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		obs.run()
	}()
	t.Cleanup(stopFn)

	obs.GetHandle("dogstatsd").ObserveMetric(&metricObs{
		name:      "system.cpu.user",
		value:     50,
		tags:      []string{"env:prod"},
		timestamp: 1000,
	})
	obs.GetHandle("check").ObserveMetric(&metricObs{
		name:      "system.cpu.user",
		value:     75,
		timestamp: 1000,
	})

	stopFn()

	assert.Empty(t, storage.ListSeries(observerdef.SeriesFilter{Namespace: "dogstatsd"}))

	checkSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: "check"})
	require.Len(t, checkSeries, 1)
	assert.Equal(t, "system.cpu.user", checkSeries[0].Name)
}

func TestIngestMetricSyncAppliesMetricFilterBySource(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type:   excludeAtMatch,
		Name:   "drop_dogstatsd",
		Source: "dogstatsd",
	}})
	require.NoError(t, err)

	storage := newTimeSeriesStorage()
	obs := &observerImpl{
		engine:       newEngine(engineConfig{storage: storage}),
		metricFilter: filter,
	}

	obs.IngestMetricSync("dogstatsd", &metricObs{
		name:      "system.cpu.user",
		value:     50,
		timestamp: 1000,
	})
	obs.IngestMetricSync("check", &metricObs{
		name:      "system.cpu.user",
		value:     75,
		timestamp: 1000,
	})

	assert.Empty(t, storage.ListSeries(observerdef.SeriesFilter{Namespace: "dogstatsd"}))

	checkSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: "check"})
	require.Len(t, checkSeries, 1)
	assert.Equal(t, "system.cpu.user", checkSeries[0].Name)
}

func TestFilteredMetricTelemetryAsyncPath(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type:   excludeAtMatch,
		Name:   "drop_dogstatsd",
		Source: "dogstatsd",
	}})
	require.NoError(t, err)

	telComp := telemetryimpl.GetCompatComponent()
	telComp.Reset()
	t.Cleanup(telComp.Reset)

	obs := &observerImpl{
		engine:               newEngine(engineConfig{storage: newTimeSeriesStorage()}),
		obsCh:                make(chan observation, 1),
		telemetry:            newObserverTelemetry(telComp),
		ingestMetricsEnabled: true,
		metricFilter:         filter,
	}
	obs.handleFunc = obs.innerHandle

	obs.GetHandle("dogstatsd").ObserveMetric(&metricObs{
		name:      "system.cpu.user",
		value:     50,
		timestamp: 1000,
	})

	requireCounterMetricValueBySource(t, "dogstatsd", 1.0, telComp)
}

func TestFilteredMetricTelemetrySyncPath(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type:   excludeAtMatch,
		Name:   "drop_check",
		Source: "check",
	}})
	require.NoError(t, err)

	telComp := telemetryimpl.GetCompatComponent()
	telComp.Reset()
	t.Cleanup(telComp.Reset)

	obs := &observerImpl{
		engine:       newEngine(engineConfig{storage: newTimeSeriesStorage()}),
		telemetry:    newObserverTelemetry(telComp),
		metricFilter: filter,
	}

	obs.IngestMetricSync("check", &metricObs{
		name:      "system.cpu.user",
		value:     75,
		timestamp: 1000,
	})

	requireCounterMetricValueBySource(t, "check", 1.0, telComp)
}

func TestDefaultFilterAsyncPathIngestsAgentMetricsAndFiltersObserverTelemetry(t *testing.T) {
	telComp := telemetryimpl.GetCompatComponent()
	telComp.Reset()
	t.Cleanup(telComp.Reset)

	defaultFilter, err := newDefaultMetricsFilterRules()
	require.NoError(t, err)

	storage := newTimeSeriesStorage()
	obs := &observerImpl{
		engine:               newEngine(engineConfig{storage: storage}),
		obsCh:                make(chan observation, 16),
		telemetry:            newObserverTelemetry(telComp),
		ingestMetricsEnabled: true,
		metricFilter:         defaultFilter,
	}
	obs.handleFunc = obs.innerHandle

	var (
		wg        sync.WaitGroup
		closeOnce sync.Once
	)
	stopFn := func() {
		closeOnce.Do(func() { close(obs.obsCh) })
		wg.Wait()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		obs.run()
	}()
	t.Cleanup(stopFn)

	obs.GetHandle("dogstatsd").ObserveMetric(&metricObs{
		name:      "system.cpu.user",
		value:     50,
		timestamp: 1000,
	})
	obs.GetHandle("check").ObserveMetric(&metricObs{
		name:      "system.mem.used",
		value:     1024,
		timestamp: 1000,
	})
	obs.GetHandle("check").ObserveMetric(&metricObs{
		name:      "datadog.agent.running",
		value:     1,
		timestamp: 1000,
	})
	obs.GetHandle("check").ObserveMetric(&metricObs{
		name:      observerTelemetryMetricPrefix + "metrics.filtered",
		value:     1,
		timestamp: 1000,
	})

	stopFn()

	dogstatsdSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: "dogstatsd"})
	require.Len(t, dogstatsdSeries, 1)
	assert.Equal(t, "system.cpu.user", dogstatsdSeries[0].Name)

	checkSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: "check"})
	require.Len(t, checkSeries, 1)
	assert.Equal(t, "system.mem.used", checkSeries[0].Name)

	agentSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: observerdef.AgentNamespace})
	require.Len(t, agentSeries, 1)
	assert.Equal(t, "datadog.agent.running", agentSeries[0].Name)

	requireCounterMetricValueBySource(t, observerdef.AgentNamespace, 1.0, telComp)
	requireNoCounterMetricForNameBySource(t, telemetryObsChannelDropped, "check", telComp)
}

func TestTagBasedFilterCountsOnlyFullyMatchingSamples(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type: excludeAtMatch,
		Name: "drop_dev_web",
		Tags: []string{"env:dev", "service:web"},
	}})
	require.NoError(t, err)

	telComp := telemetryimpl.GetCompatComponent()
	telComp.Reset()
	t.Cleanup(telComp.Reset)

	storage := newTimeSeriesStorage()
	obs := &observerImpl{
		engine:       newEngine(engineConfig{storage: storage}),
		telemetry:    newObserverTelemetry(telComp),
		metricFilter: filter,
	}

	obs.IngestMetricSync("dogstatsd", &metricObs{
		name:      "system.cpu.user",
		value:     1,
		tags:      []string{"env:dev", "service:web"},
		timestamp: 1000,
	})
	obs.IngestMetricSync("dogstatsd", &metricObs{
		name:      "system.cpu.user",
		value:     2,
		tags:      []string{"env:dev"},
		timestamp: 1000,
	})
	obs.IngestMetricSync("dogstatsd", &metricObs{
		name:      "system.cpu.user",
		value:     3,
		tags:      []string{"service:web"},
		timestamp: 1000,
	})

	dogstatsdSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: "dogstatsd"})
	require.Len(t, dogstatsdSeries, 2)

	requireCounterMetricValueBySource(t, "dogstatsd", 1.0, telComp)
}

func TestNamePrefixFilterCountsFilteredMetrics(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type:        excludeAtMatch,
		Name:        "drop_kubernetes",
		NamePattern: "kubernetes.*",
	}})
	require.NoError(t, err)

	telComp := telemetryimpl.GetCompatComponent()
	telComp.Reset()
	t.Cleanup(telComp.Reset)

	storage := newTimeSeriesStorage()
	obs := &observerImpl{
		engine:       newEngine(engineConfig{storage: storage}),
		telemetry:    newObserverTelemetry(telComp),
		metricFilter: filter,
	}

	obs.IngestMetricSync("check", &metricObs{
		name:      "kubernetes.cpu.usage",
		value:     1,
		timestamp: 1000,
	})
	obs.IngestMetricSync("check", &metricObs{
		name:      "system.cpu.user",
		value:     2,
		timestamp: 1000,
	})

	checkSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: "check"})
	require.Len(t, checkSeries, 1)
	assert.Equal(t, "system.cpu.user", checkSeries[0].Name)

	requireCounterMetricValueBySource(t, "check", 1.0, telComp)
}

func TestMixedAgentRulesAsyncPathKeepsIncludedMetricAndCountsDroppedMetric(t *testing.T) {
	filter, err := newMetricsFilterRules(append([]metricsProcessingRule{
		{
			Type:        includeAtMatch,
			Name:        "keep_agent_running",
			NamePattern: "datadog.agent.running",
			Source:      observerdef.AgentNamespace,
		},
		{
			Type:   excludeAtMatch,
			Name:   "drop_other_agent_metrics",
			Source: observerdef.AgentNamespace,
		},
	}, implicitMetricsProcessingRules()...))
	require.NoError(t, err)

	telComp := telemetryimpl.GetCompatComponent()
	telComp.Reset()
	t.Cleanup(telComp.Reset)

	storage := newTimeSeriesStorage()
	obs := &observerImpl{
		engine:               newEngine(engineConfig{storage: storage}),
		obsCh:                make(chan observation, 16),
		telemetry:            newObserverTelemetry(telComp),
		ingestMetricsEnabled: true,
		metricFilter:         filter,
	}
	obs.handleFunc = obs.innerHandle

	var (
		wg        sync.WaitGroup
		closeOnce sync.Once
	)
	stopFn := func() {
		closeOnce.Do(func() { close(obs.obsCh) })
		wg.Wait()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		obs.run()
	}()
	t.Cleanup(stopFn)

	h := obs.GetHandle("dogstatsd")
	h.ObserveMetric(&metricObs{
		name:      "datadog.agent.running",
		value:     1,
		timestamp: 1000,
	})
	h.ObserveMetric(&metricObs{
		name:      "datadog.agent.uptime",
		value:     1,
		timestamp: 1000,
	})

	stopFn()

	agentSeries := storage.ListSeries(observerdef.SeriesFilter{Namespace: observerdef.AgentNamespace})
	require.Len(t, agentSeries, 1)
	assert.Equal(t, "datadog.agent.running", agentSeries[0].Name)

	requireCounterMetricValueBySource(t, observerdef.AgentNamespace, 1.0, telComp)
}

func TestAsyncAndSyncFilteringForCheckSourceRemainConsistent(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type:   excludeAtMatch,
		Name:   "drop_check",
		Source: "check",
	}})
	require.NoError(t, err)

	telComp := telemetryimpl.GetCompatComponent()
	telComp.Reset()
	t.Cleanup(telComp.Reset)

	storage := newTimeSeriesStorage()
	obs := &observerImpl{
		engine:               newEngine(engineConfig{storage: storage}),
		obsCh:                make(chan observation, 16),
		telemetry:            newObserverTelemetry(telComp),
		ingestMetricsEnabled: true,
		metricFilter:         filter,
	}
	obs.handleFunc = obs.innerHandle

	var (
		wg        sync.WaitGroup
		closeOnce sync.Once
	)
	stopFn := func() {
		closeOnce.Do(func() { close(obs.obsCh) })
		wg.Wait()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		obs.run()
	}()
	t.Cleanup(stopFn)

	obs.GetHandle("check").ObserveMetric(&metricObs{
		name:      "system.cpu.user",
		value:     1,
		timestamp: 1000,
	})
	obs.IngestMetricSync("check", &metricObs{
		name:      "system.mem.used",
		value:     2,
		timestamp: 1000,
	})

	stopFn()

	assert.Empty(t, storage.ListSeries(observerdef.SeriesFilter{Namespace: "check"}))
	requireCounterMetricValueBySource(t, "check", 2.0, telComp)
}

// --- containsAllTagsSorted ---

func TestContainsAllTagsSorted_EmptyRuleTags(t *testing.T) {
	assert.True(t, containsAllTagsSorted(nil, nil))
	assert.True(t, containsAllTagsSorted([]string{"env:prod"}, nil))
}

func TestContainsAllTagsSorted_EmptySampleTags(t *testing.T) {
	assert.False(t, containsAllTagsSorted(nil, []string{"env:prod"}))
}

func TestContainsAllTagsSorted_AllMatch(t *testing.T) {
	sample := []string{"env:prod", "service:web", "team:foo"}
	rule := []string{"env:prod", "service:web"}
	assert.True(t, containsAllTagsSorted(sample, rule))
}

func TestContainsAllTagsSorted_PartialMatch(t *testing.T) {
	sample := []string{"env:prod", "service:web"}
	rule := []string{"env:prod", "team:foo"}
	assert.False(t, containsAllTagsSorted(sample, rule))
}

func TestContainsAllTagsSorted_SampleExhaustedBeforeAllRuleTags(t *testing.T) {
	sample := []string{"a:1"}
	rule := []string{"a:1", "z:9"}
	assert.False(t, containsAllTagsSorted(sample, rule))
}

func TestContainsAllTagsSorted_ExactMatch(t *testing.T) {
	tags := []string{"env:dev", "service:api"}
	assert.True(t, containsAllTagsSorted(tags, tags))
}

// --- compileRuleTags deduplication ---

func TestCompileRuleTagsDeduplicate(t *testing.T) {
	compiled, err := compileRuleTags([]string{"env:prod", "env:prod", "service:web"})
	require.NoError(t, err)
	assert.Equal(t, []string{"env:prod", "service:web"}, compiled)
}

func TestMetricsFilterRulesDuplicateRuleTagsBehaveAsIfUnique(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type: excludeAtMatch,
		Name: "drop_prod",
		Tags: []string{"env:prod", "env:prod"},
	}})
	require.NoError(t, err)

	// Should match the same as a rule with a single "env:prod".
	assert.False(t, filter.isAllowed("system.cpu.user", "dogstatsd", []string{"env:prod"}))
	assert.True(t, filter.isAllowed("system.cpu.user", "dogstatsd", []string{"env:dev"}))
}

func TestFilteredMetricsAndChannelDropsIncrementSeparateCounters(t *testing.T) {
	filter, err := newMetricsFilterRules([]metricsProcessingRule{{
		Type:        excludeAtMatch,
		Name:        "drop_system_cpu",
		NamePattern: "system.cpu.",
	}})
	require.NoError(t, err)

	telComp := telemetryimpl.GetCompatComponent()
	telComp.Reset()
	t.Cleanup(telComp.Reset)

	obs := &observerImpl{
		engine:               newEngine(engineConfig{storage: newTimeSeriesStorage()}),
		obsCh:                make(chan observation, 1),
		telemetry:            newObserverTelemetry(telComp),
		ingestMetricsEnabled: true,
		metricFilter:         filter,
	}
	obs.handleFunc = obs.innerHandle

	h, ok := obs.GetHandle("dogstatsd").(*handle)
	require.True(t, ok)
	assert.False(t, h.ObserveMetricAndReportDrop(&metricObs{
		name:      "system.mem.used",
		value:     1,
		timestamp: 1000,
	}))
	assert.True(t, h.ObserveMetricAndReportDrop(&metricObs{
		name:      "kubernetes.cpu.usage",
		value:     2,
		timestamp: 1000,
	}))
	assert.False(t, h.ObserveMetricAndReportDrop(&metricObs{
		name:      "system.cpu.user",
		value:     3,
		timestamp: 1000,
	}))

	requireCounterMetricValueForNameBySource(t, telemetryObsChannelDropped, "dogstatsd", 1.0, telComp)
	requireCounterMetricValueBySource(t, "dogstatsd", 1.0, telComp)
}
