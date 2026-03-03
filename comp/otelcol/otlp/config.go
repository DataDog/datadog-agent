// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

//go:build otlp

package otlp

import (
	"context"
	"errors"
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

// errors for ADP OTLP proxy configuration validation
var (
	ErrProxyGRPCEndpointNotConfigured = errors.New("data plane OTLP proxy enabled but gRPC endpoint is not configured")
	ErrProxyGRPCEndpointCollision     = errors.New("data plane OTLP proxy gRPC endpoint conflicts with receiver endpoint")
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

	proxyEnabled := cfg.GetBool(coreconfig.DataPlaneOTLPProxyEnabled)

	otlpReceiverConfig := configcheck.ReadConfigSection(cfg, coreconfig.OTLPReceiverSection)
	otlpReceiverConfigMap := otlpReceiverConfig.ToStringMap()

	if proxyEnabled {
		// ADP OTLP proxy will own the configured gRPC endpoint, so we need to assign a different endpoint to be used by the core agent
		protocols, ok := otlpReceiverConfigMap["protocols"].(map[string]interface{})
		if !ok {
			return PipelineConfig{}, errors.New("data plane OTLP proxy enabled but receiver protocols not configured")
		}
		grpc, ok := protocols["grpc"].(map[string]interface{})
		if !ok {
			return PipelineConfig{}, errors.New("data plane OTLP proxy enabled but gRPC protocol not configured")
		}

		proxyGRPCEndpoint := cfg.GetString(coreconfig.DataPlaneOTLPProxyReceiverProtocolsGRPCEndpoint)
		originalGRPCEndpoint, _ := grpc["endpoint"].(string)

		if proxyGRPCEndpoint == "" {
			errs = append(errs, ErrProxyGRPCEndpointNotConfigured)
		}

		// Check for endpoint collision
		if proxyGRPCEndpoint != "" && proxyGRPCEndpoint == originalGRPCEndpoint {
			errs = append(errs, fmt.Errorf("%w: %q", ErrProxyGRPCEndpointCollision, proxyGRPCEndpoint))
		}

		grpc["endpoint"] = proxyGRPCEndpoint
	}

	tracePort, err := portToUint(cfg.GetInt(coreconfig.OTLPTracePort))
	if err != nil {
		errs = append(errs, fmt.Errorf("internal trace port is invalid: %w", err))
	}
	metricsEnabled := cfg.GetBool(coreconfig.OTLPMetricsEnabled)
	tracesEnabled := cfg.GetBool(coreconfig.OTLPTracesEnabled)
	logsEnabled := cfg.GetBool(coreconfig.OTLPLogsEnabled)
	TracesInfraAttributesEnabled := cfg.GetBool(coreconfig.OTLPTracesInfraAttrEnabled)

	if !metricsEnabled && !tracesEnabled && !logsEnabled {
		errs = append(errs, errors.New("at least one OTLP signal needs to be enabled"))
	}

	logsConfig := configcheck.ReadConfigSection(cfg, coreconfig.OTLPLogs)

	metricsConfig := configcheck.ReadConfigSection(cfg, coreconfig.OTLPMetrics)
	metricsConfigMap := metricsConfig.ToStringMap()

	metricsBatchConfig := configcheck.ReadConfigSection(cfg, coreconfig.OTLPMetricsBatch)

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
		OTLPReceiverConfig:           otlpReceiverConfigMap,
		TracePort:                    tracePort,
		MetricsEnabled:               metricsEnabled,
		TracesEnabled:                tracesEnabled,
		LogsEnabled:                  logsEnabled,
		Metrics:                      mc,
		TracesInfraAttributesEnabled: TracesInfraAttributesEnabled,
		MetricsBatch:                 metricsBatchConfig.ToStringMap(),
		Logs:                         logsConfig.ToStringMap(),
		Debug:                        debugConfig.ToStringMap(),
	}, multierr.Combine(errs...)
}

func normalizeMetricsConfig(metricsConfigMap map[string]interface{}, strict bool) (map[string]interface{}, error) {
	// metricsConfigMap doesn't strictly match the types present in MetricsConfig struct
	// so to get properly type map we need to decode it twice

	// We need to start with default config to get the corrent default values
	cf := serializerexporter.NewFactoryForAgent(nil, nil, serializerexporter.TelemetryStore{}).CreateDefaultConfig()

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
