// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp

package otlp

import (
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/confmap"
	"go.uber.org/multierr"

	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/internal/configutils"
)

// buildKey creates a key for referencing a nested field.
func buildKey(keys ...string) string {
	return strings.Join(keys, confmap.KeyDelimiter)
}

func buildTracesMap(cfg PipelineConfig) (*confmap.Conf, error) {
	baseMap, err := configutils.NewMapFromYAMLString(defaultTracesConfig)
	if err != nil {
		return nil, err
	}

	// Remove infraattributes if disabled
	if !cfg.TracesInfraAttributesEnabled {
		if err := removeInfraAttributesProcessor(baseMap, "traces"); err != nil {
			return nil, err
		}
	}

	smap := map[string]interface{}{
		buildKey("exporters", "otlp", "endpoint"): fmt.Sprintf("%s:%d", "localhost", cfg.TracePort),
	}
	{
		configMap := confmap.NewFromStringMap(smap)
		err = baseMap.Merge(configMap)
	}
	return baseMap, err
}

// ensureNonNilMap converts a nil map to an empty map.
// This ensures consistent behavior when merging configurations.
func ensureNonNilMap(m map[string]interface{}) map[string]interface{} {
	if m == nil {
		return map[string]interface{}{}
	}
	return m
}

func buildMetricsMap(cfg PipelineConfig) (*confmap.Conf, error) {
	baseMap, err := configutils.NewMapFromYAMLString(defaultMetricsConfig)
	if err != nil {
		return nil, err
	}
	smap := map[string]interface{}{
		buildKey("exporters", "serializer", "metrics"):                cfg.Metrics,
		buildKey("exporters", "serializer", "sending_queue", "batch"): ensureNonNilMap(cfg.MetricsBatch),
	}
	{
		configMap := confmap.NewFromStringMap(smap)
		err = baseMap.Merge(configMap)
	}
	return baseMap, err
}

func buildLogsMap(cfg PipelineConfig) (*confmap.Conf, error) {
	baseMap, err := configutils.NewMapFromYAMLString(defaultLogsConfig)
	if err != nil {
		return nil, err
	}

	smap := map[string]interface{}{
		buildKey("exporters", "logsagent", "sending_queue", "batch"): ensureNonNilMap(cfg.Logs)["batch"],
	}

	{
		configMap := confmap.NewFromStringMap(smap)
		err = baseMap.Merge(configMap)
	}

	return baseMap, err
}

func buildReceiverMap(cfg PipelineConfig) *confmap.Conf {
	rcvs := map[string]interface{}{
		"otlp": cfg.OTLPReceiverConfig,
	}
	return confmap.NewFromStringMap(map[string]interface{}{"receivers": rcvs})
}

// removeInfraAttributesProcessor removes the infraattributes processor from the pipeline config
func removeInfraAttributesProcessor(cfg *confmap.Conf, pipelineType string) error {
	// Remove from processors section
	processorsKey := buildKey("service", "pipelines", pipelineType, "processors")
	if processors, ok := cfg.Get(processorsKey).([]interface{}); ok {
		filtered := make([]interface{}, 0, len(processors))
		for _, p := range processors {
			if p != "infraattributes" {
				filtered = append(filtered, p)
			}
		}
		return cfg.Merge(confmap.NewFromStringMap(map[string]interface{}{
			processorsKey: filtered,
		}))
	}
	return nil
}

func buildMap(cfg PipelineConfig) (*confmap.Conf, error) {
	retMap := confmap.New()
	var errs []error
	if cfg.TracesEnabled {
		traceMap, err := buildTracesMap(cfg)
		errs = append(errs, err)

		err = retMap.Merge(traceMap)
		errs = append(errs, err)
	}
	if cfg.MetricsEnabled {
		metricsMap, err := buildMetricsMap(cfg)
		errs = append(errs, err)

		err = retMap.Merge(metricsMap)
		errs = append(errs, err)
	}
	if cfg.LogsEnabled {
		logsMap, err := buildLogsMap(cfg)
		errs = append(errs, err)

		err = retMap.Merge(logsMap)
		errs = append(errs, err)
	}
	if cfg.shouldSetLoggingSection() {
		m := map[string]interface{}{
			"exporters": map[string]interface{}{
				"debug": cfg.Debug,
			},
		}
		if cfg.MetricsEnabled {
			key := buildKey("service", "pipelines", "metrics", "exporters")
			if v, ok := retMap.Get(key).([]interface{}); ok {
				m[key] = append(v, "debug")
			} else {
				m[key] = []interface{}{"debug"}
			}
		}
		if cfg.TracesEnabled {
			key := buildKey("service", "pipelines", "traces", "exporters")
			if v, ok := retMap.Get(key).([]interface{}); ok {
				m[key] = append(v, "debug")
			} else {
				m[key] = []interface{}{"debug"}
			}
		}
		if cfg.LogsEnabled {
			key := buildKey("service", "pipelines", "logs", "exporters")
			if v, ok := retMap.Get(key).([]interface{}); ok {
				m[key] = append(v, "debug")
			} else {
				m[key] = []interface{}{"debug"}
			}
		}
		errs = append(errs, retMap.Merge(confmap.NewFromStringMap(m)))
	}

	err := retMap.Merge(buildReceiverMap(cfg))
	errs = append(errs, err)

	return retMap, multierr.Combine(errs...)
}
