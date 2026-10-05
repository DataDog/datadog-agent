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
	// An empty value is left unset so the processor falls back to its own default ("off").
	if cfg.TracesContainerTagPromotion != "" {
		smap[buildKey("processors", "infraattributes/traces")] = map[string]interface{}{
			"trace_container_tag_promotion": cfg.TracesContainerTagPromotion,
		}
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
	// NOTE: metrics_attributes_as_tags is intentionally NOT set here. The metrics
	// and logs pipelines share a single `infraattributes` processor instance
	// (see defaultMetricsConfig/defaultLogsConfig), and both default configs
	// declare an empty `infraattributes:` block whose nil value clobbers any
	// previously-merged option during confmap merge. buildMap therefore applies
	// the shared processor options as a final merge; see buildInfraAttributesMap.
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
	// NOTE: logs_tags_as_ddtags is intentionally NOT set here. See the note in
	// buildMetricsMap: the shared `infraattributes` processor options are applied
	// as a final merge in buildMap (see buildInfraAttributesMap).

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

// removeInfraAttributesProcessor removes any infraattributes processor
// instance (base `infraattributes` or a named variant such as
// `infraattributes/traces`) from the given pipeline's processor list.
func removeInfraAttributesProcessor(cfg *confmap.Conf, pipelineType string) error {
	// Remove from processors section
	processorsKey := buildKey("service", "pipelines", pipelineType, "processors")
	if processors, ok := cfg.Get(processorsKey).([]interface{}); ok {
		filtered := make([]interface{}, 0, len(processors))
		for _, p := range processors {
			name, _ := p.(string)
			if name == "infraattributes" || strings.HasPrefix(name, "infraattributes/") {
				continue
			}
			filtered = append(filtered, p)
		}
		return cfg.Merge(confmap.NewFromStringMap(map[string]interface{}{
			processorsKey: filtered,
		}))
	}
	return nil
}

// buildInfraAttributesMap returns the options for the `infraattributes`
// processor shared by the metrics and logs pipelines, or nil if none are set.
//
// These options MUST be applied after the per-pipeline maps are merged. The
// metrics and logs default configs each declare an empty `infraattributes:`
// block, and confmap/koanf overwrites a map value with a nil one when merging,
// so setting an option inside buildMetricsMap/buildLogsMap lets whichever
// pipeline is merged last (logs) clobber the other's option (OTELS-1131).
// Applying them last, as a real (non-nil) map, is safe regardless of order.
func buildInfraAttributesMap(cfg PipelineConfig) *confmap.Conf {
	opts := map[string]interface{}{}
	if cfg.MetricsEnabled && cfg.MetricsInfraAttrsAsTags {
		opts["metrics_attributes_as_tags"] = true
	}
	if cfg.LogsEnabled && cfg.LogsTagsAsDDTags {
		opts["logs_tags_as_ddtags"] = true
	}
	if len(opts) == 0 {
		return nil
	}
	return confmap.NewFromStringMap(map[string]interface{}{
		buildKey("processors", "infraattributes"): opts,
	})
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

	// Apply the shared infraattributes processor options last so that neither
	// the metrics nor the logs pipeline's empty `infraattributes:` block can
	// clobber them during merge (see buildInfraAttributesMap).
	if infraAttrsMap := buildInfraAttributesMap(cfg); infraAttrsMap != nil {
		errs = append(errs, retMap.Merge(infraAttrsMap))
	}

	return retMap, multierr.Combine(errs...)
}
