// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp

package otlp

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/multierr"

	"github.com/go-viper/mapstructure/v2"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/configcheck"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
	tagutil "github.com/DataDog/datadog-agent/pkg/util/tags"
)

func portToUint(v int) (port uint, err error) {
	if v < 0 || v > 65535 {
		err = fmt.Errorf("%d is out of [0, 65535] range", v)
	}
	port = uint(v)
	return
}

// FromAgentConfig builds a pipeline configuration from an Agent configuration.
func FromAgentConfig(cfg config.Reader) (PipelineConfig, error) {
	var errs []error
	otlpConfig := configcheck.ReadConfigSection(cfg, coreconfig.OTLPReceiverSection)
	tracePort, err := portToUint(cfg.GetInt(coreconfig.OTLPTracePort))
	if err != nil {
		errs = append(errs, fmt.Errorf("internal trace port is invalid: %w", err))
	}
	metricsEnabled := cfg.GetBool(coreconfig.OTLPMetricsEnabled)
	tracesEnabled := cfg.GetBool(coreconfig.OTLPTracesEnabled)
	logsEnabled := cfg.GetBool(coreconfig.OTLPLogsEnabled)
	if !metricsEnabled && !tracesEnabled && !logsEnabled {
		errs = append(errs, fmt.Errorf("at least one OTLP signal needs to be enabled"))
	}
	metricsConfig := configcheck.ReadConfigSection(cfg, coreconfig.OTLPMetrics)
	metricsConfigMap := metricsConfig.ToStringMap()

	if _, ok := metricsConfigMap["apm_stats_receiver_addr"]; !ok {
		metricsConfigMap["apm_stats_receiver_addr"] = fmt.Sprintf("http://localhost:%s/v0.6/stats", coreconfig.Datadog().GetString("apm_config.receiver_port"))
	}

	tags := strings.Join(tagutil.GetStaticTagsSlice(context.TODO(), cfg), ",")
	if tags != "" {
		metricsConfigMap["tags"] = tags
	}
	mc, err := normalizeMetricsConfig(metricsConfigMap, false)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to normalize metrics config: %w", err))
	}

	debugConfig := configcheck.ReadConfigSection(cfg, coreconfig.OTLPDebug)

	return PipelineConfig{
		OTLPReceiverConfig: otlpConfig.ToStringMap(),
		TracePort:          tracePort,
		MetricsEnabled:     metricsEnabled,
		TracesEnabled:      tracesEnabled,
		LogsEnabled:        logsEnabled,
		Metrics:            mc,
		Debug:              debugConfig.ToStringMap(),
	}, multierr.Combine(errs...)
}

func normalizeMetricsConfig(metricsConfigMap map[string]interface{}, strict bool) (map[string]interface{}, error) {
	// metricsConfigMap doesn't strictly match the types present in MetricsConfig struct
	// so to get properly type map we need to decode it twice

	// We need to start with default config to get the corrent default values
	cf := serializerexporter.NewFactoryForAgent(nil, nil, nil).CreateDefaultConfig()

	x := cf.(*serializerexporter.ExporterConfig).Metrics
	if strict {
		err := mapstructure.Decode(metricsConfigMap, &x)
		if err != nil {
			return nil, err
		}
	} else {

		err := mapstructure.WeakDecode(metricsConfigMap, &x)
		if err != nil {
			return nil, err
		}
	}
	mc := make(map[string]interface{})
	err := mapstructure.WeakDecode(x, &mc)
	if err != nil {
		return nil, err
	}
	return mc, nil
}
