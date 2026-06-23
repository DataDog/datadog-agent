// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package converters

// TODO: refactor Prometheus common code with ddot

import (
	"errors"
	"path"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/confmaputils"
	"github.com/DataDog/datadog-agent/pkg/util/hostport"
)

const (
	// OpenTelemetry Collector defaults.
	// https://github.com/open-telemetry/opentelemetry-go-contrib/blob/v1.39.0/otelconf/v0.3.0/metric.go#L345
	defaultTelemetryReaderTarget = "localhost:8888"
	defaultTelemetryReaderScheme = "http"
	otelDefaultMetricsPath       = "/metrics"

	// Prometheus defaults.
	// https://prometheus.io/docs/prometheus/latest/configuration/configuration/#scrape_config
	prometheusDefaultMetricsPath = "/metrics"
)

// We don't want to error out in case of invalid user-config, these ones are just logged and ignored.

// errInvalidTelemetryMetricsReaders is returned when service::telemetry::metrics::readers is
// present but empty, or no prometheus reader with host and port could be found.
var errInvalidTelemetryMetricsReaders = errors.New("invalid telemetry::metrics::readers")

// errInvalidPrometheusStaticConfigs is returned when a scrape_config static_configs entry has an
// unexpected type.
var errInvalidPrometheusStaticConfigs = errors.New("invalid prometheus::scrape_config::static_configs")

// errInvalidPrometheusReceiver is returned when getCoveredExportersInMetricsPipelines
// cannot find a valid prometheus receiver config.
var errInvalidPrometheusReceiver = errors.New("invalid prometheus receiver config")

// prometheusTelemetryTarget is the host:port scraped for collector internal metrics
// and the url scheme of that endpoint.
type prometheusTelemetryTarget struct {
	HostPort string
	Scheme   string
}

func telemetryTargetSet(targets []prometheusTelemetryTarget) map[prometheusTelemetryTarget]struct{} {
	out := make(map[prometheusTelemetryTarget]struct{}, len(targets))
	for _, t := range targets {
		k := prometheusTelemetryTarget{
			HostPort: t.HostPort,
			Scheme:   strings.ToLower(strings.TrimSpace(t.Scheme)),
		}
		out[k] = struct{}{}
	}
	return out
}

// selectTelemetryPrometheusTargets returns the list of telemetry prometheus targets from the config.
func selectTelemetryPrometheusTargets(conf confMap) ([]prometheusTelemetryTarget, error) {
	metrics, ok := confmaputils.Get[confMap](conf, "service::telemetry::metrics")
	if !ok {
		return []prometheusTelemetryTarget{{HostPort: defaultTelemetryReaderTarget, Scheme: defaultTelemetryReaderScheme}}, nil
	}
	readers, exists := metrics["readers"]
	if !exists {
		return []prometheusTelemetryTarget{{HostPort: defaultTelemetryReaderTarget, Scheme: defaultTelemetryReaderScheme}}, nil
	}

	readersSlice, ok := readers.([]any)
	if !ok || len(readersSlice) == 0 {
		return nil, errInvalidTelemetryMetricsReaders
	}

	targets := make([]prometheusTelemetryTarget, 0, len(readersSlice))
	for _, reader := range readersSlice {
		readerConfMap, ok := reader.(confMap)
		if !ok || len(readerConfMap) != 1 {
			continue
		}

		prometheusExporter, ok := confmaputils.Get[confMap](readerConfMap, "pull::exporter::prometheus")
		if !ok {
			continue
		}

		host, hostOk := confmaputils.Get[string](prometheusExporter, "host")
		port, portOk := confmaputils.Get[int](prometheusExporter, "port")
		if !hostOk || !portOk {
			continue
		}

		targets = append(targets, prometheusTelemetryTarget{
			HostPort: hostport.Join(host, strconv.Itoa(port)),
			Scheme:   defaultTelemetryReaderScheme,
		})
	}

	if len(targets) == 0 {
		return nil, errInvalidTelemetryMetricsReaders
	}
	return targets, nil
}

func scrapeTargetsMatchTelemetry(staticConfigs []any, telemetryEndpoints map[prometheusTelemetryTarget]struct{}, scrapeScheme string) (covers bool, err error) {
	for _, staticConfig := range staticConfigs {
		staticConfigMap, cfgOk := staticConfig.(confMap)
		if !cfgOk {
			return false, errInvalidPrometheusStaticConfigs
		}
		targets, exists := staticConfigMap["targets"]
		if !exists {
			continue
		}
		targetsSlice, tsOk := targets.([]any)
		if !tsOk {
			return false, errInvalidPrometheusStaticConfigs
		}
		for _, target := range targetsSlice {
			targetString, tOk := target.(string)
			if !tOk {
				return false, errInvalidPrometheusStaticConfigs
			}
			if _, ok := telemetryEndpoints[prometheusTelemetryTarget{HostPort: targetString, Scheme: scrapeScheme}]; ok {
				return true, nil
			}
		}
	}
	return false, nil
}

// getCoveredExportersInMetricsPipelines identifies profile exporters that already
// receive the Collector's internal telemetry through a user-defined metrics pipeline.
//
// A scrape job counts as covering only when host:port, scheme, and metrics_path
// match the OTel internal endpoint.
//
// For each prometheus receiver, scheme and metrics_path are evaluated per scrape_config;
// when any scrape_config matches telemetry, service pipelines are scanned at most once
// for that receiver and matching exporters are merged into coveredExporters.
func getCoveredExportersInMetricsPipelines(conf confMap, telemetryTargets []prometheusTelemetryTarget, filter func(string) bool) (map[string]struct{}, error) {
	coveredExporters := make(map[string]struct{})
	pipelines, _ := confmaputils.Get[confMap](conf, "service::pipelines")
	telemetryEndpoints := telemetryTargetSet(telemetryTargets)

	cfgsByReceiver := findComponentConfigs(conf, "receivers", "prometheus")
	for receiverName, receiver := range cfgsByReceiver {
		prometheusConfig, ok := receiver["config"]
		if !ok {
			continue
		}
		prometheusConfigMap, ok := prometheusConfig.(confMap)
		if !ok {
			return nil, errInvalidPrometheusReceiver
		}
		prometheusScrapeConfigs, ok := prometheusConfigMap["scrape_configs"]
		if !ok {
			continue
		}
		prometheusScrapeConfigsSlice, ok := prometheusScrapeConfigs.([]any)
		if !ok {
			return nil, errInvalidPrometheusReceiver
		}
		anyScrapeCoversTelemetry := false
		for _, scrapeConfig := range prometheusScrapeConfigsSlice {
			scrapeConfigMap, ok := scrapeConfig.(confMap)
			if !ok {
				return nil, errInvalidPrometheusReceiver
			}

			// check if the metrics path is the same as the default OTel metrics path
			metricsPath, ok := normalizePrometheusMetricsPath(scrapeConfigMap)
			if !ok {
				return nil, errInvalidPrometheusReceiver
			}
			if metricsPath != otelDefaultMetricsPath {
				continue
			}

			// check if the scheme is the same as the default OTel scheme
			scrapeScheme := defaultTelemetryReaderScheme
			if s, ok := confmaputils.Get[string](scrapeConfigMap, "scheme"); ok {
				scrapeScheme = s
			}
			scheme := strings.ToLower(strings.TrimSpace(scrapeScheme))

			// check if the static_configs targets match the telemetry targets
			staticConfigSlice, ok := confmaputils.Get[[]any](scrapeConfigMap, "static_configs")
			if !ok {
				continue
			}
			scrapeCoversTelemetry, err := scrapeTargetsMatchTelemetry(staticConfigSlice, telemetryEndpoints, scheme)
			if err != nil {
				return nil, err
			}
			if scrapeCoversTelemetry {
				anyScrapeCoversTelemetry = true
			}
		}
		if anyScrapeCoversTelemetry {
			addExportersInSamePipelineAsReceiver(pipelines, receiverName, filter, coveredExporters)
		}
	}
	return coveredExporters, nil
}

func addExportersInSamePipelineAsReceiver(pipelines confMap, receiverName string, filter func(string) bool, coveredExporters map[string]struct{}) {
	for pipelineName, pipeline := range pipelines {
		if !confmaputils.IsComponentType(pipelineName, "metrics") {
			continue
		}
		pipelineConfMap, ok := pipeline.(confMap)
		if !ok {
			continue
		}
		hasReceiver := false
		forEachComponentUntil(pipelineConfMap, "receivers", func(s string) bool {
			if s == receiverName {
				hasReceiver = true
				return false
			}
			return true
		})
		if !hasReceiver {
			continue
		}
		forEachComponentUntil(pipelineConfMap, "exporters", func(s string) bool {
			if filter != nil && !filter(s) {
				return true
			}
			coveredExporters[s] = struct{}{}
			return true
		})
	}
}

func normalizePrometheusMetricsPath(scrapeConfigMap confMap) (string, bool) {
	raw, exists := scrapeConfigMap["metrics_path"]
	if !exists {
		return prometheusDefaultMetricsPath, true
	}
	s, ok := raw.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return prometheusDefaultMetricsPath, true
	}
	return path.Clean(s), true
}

// forEachComponentUntil invokes fn for each string entry under componentKind (e.g. "receivers", "exporters").
// Non-string entries are skipped. If fn returns false, iteration stops immediately.
func forEachComponentUntil(pipelineConfMap confMap, componentKind string, fn func(string) bool) {
	list, ok := confmaputils.Get[[]any](pipelineConfMap, componentKind)
	if !ok {
		return
	}
	for _, item := range list {
		s, ok := item.(string)
		if !ok {
			continue
		}
		if !fn(s) {
			return
		}
	}
}

// findComponentConfigs returns configs for components of the given componentType
// (e.g. "prometheus") under the top-level componentKind section (e.g. "receivers").
func findComponentConfigs(conf confMap, componentKind string, componentType string) map[string]confMap {
	componentsMap, ok := confmaputils.Get[confMap](conf, componentKind)
	if !ok {
		return nil
	}

	configsByComponent := make(map[string]confMap)
	for name, cfg := range componentsMap {
		if !confmaputils.IsComponentType(name, componentType) {
			continue
		}
		cfgMap, ok := cfg.(confMap)
		if !ok {
			cfgMap = nil // some components can leave configs empty and use defaults
		}
		configsByComponent[name] = cfgMap
	}
	return configsByComponent
}
