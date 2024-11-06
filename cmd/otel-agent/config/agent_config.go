// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config provides a way to convert the OpenTelemetry Collector configuration to the Datadog Agent configuration.
package config

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpsprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/service"
)

type logLevel int

const (
	trace logLevel = iota - 1
	debug
	info
	warn
	err
	critical
	off
)

// datadog agent log levels: trace, debug, info, warn, error, critical, and off
// otel log levels: disabled, debug, info, warn, error
var logLevelMap = map[string]logLevel{
	"off":      off,
	"disabled": off,
	"trace":    trace,
	"debug":    debug,
	"info":     info,
	"warn":     warn,
	"error":    err,
	"critical": critical,
}

var logLevelReverseMap = func(src map[string]logLevel) map[logLevel]string {
	reverse := map[logLevel]string{}
	for k, v := range src {
		reverse[v] = k
	}

	return reverse
}(logLevelMap)

// ErrNoDDExporter indicates there is no Datadog exporter in the configs
var ErrNoDDExporter = fmt.Errorf("no datadog exporter found")

// NewConfigComponent creates a new config component from the given URIs
func NewConfigComponent(ctx context.Context, ddCfg string, uris []string) (config.Component, error) {
	if len(uris) == 0 {
		return nil, errors.New("no URIs provided for configs")
	}
	// Load the configuration from the fileName
	rs := confmap.ResolverSettings{
		URIs: uris,
		ProviderFactories: []confmap.ProviderFactory{
			fileprovider.NewFactory(),
			envprovider.NewFactory(),
			yamlprovider.NewFactory(),
			httpprovider.NewFactory(),
			httpsprovider.NewFactory(),
		},
		ConverterFactories: []confmap.ConverterFactory{expandconverter.NewFactory()},
	}

	resolver, err := confmap.NewResolver(rs)
	if err != nil {
		return nil, err
	}
	cfg, err := resolver.Resolve(ctx)
	if err != nil {
		return nil, err
	}
	sc, err := getServiceConfig(cfg)
	if err != nil {
		return nil, err
	}

	// Set the global agent config
	pkgconfig := pkgconfigsetup.Datadog()

	pkgconfig.SetConfigName("OTel")
	pkgconfig.SetEnvPrefix("DD")
	pkgconfig.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	pkgconfig.BindEnvAndSetDefault("log_level", "info")

	activeLogLevel := critical
	if len(ddCfg) != 0 {
		// if the configuration file path was supplied via CLI flags or env vars,
		// add that first so it's first in line
		pkgconfig.AddConfigPath(ddCfg)
		// If they set a config file directly, let's try to honor that
		if strings.HasSuffix(ddCfg, ".yaml") || strings.HasSuffix(ddCfg, ".yml") {
			pkgconfig.SetConfigFile(ddCfg)
		}

		_, err = pkgconfigsetup.LoadWithoutSecret(pkgconfig, nil)
		if err != nil {
			return nil, err
		}
		var ok bool
		activeLogLevel, ok = logLevelMap[pkgconfig.GetString("log_level")]
		if !ok {
			return nil, fmt.Errorf("invalid log level (%v) set in the Datadog Agent configuration", pkgconfig.GetString("log_level"))
		}
	}

	// Set the right log level. The most verbose setting takes precedence.
	telemetryLogLevel := sc.Telemetry.Logs.Level
	telemetryLogMapping, ok := logLevelMap[telemetryLogLevel.String()]
	if !ok {
		return nil, fmt.Errorf("invalid log level (%v) set in the OTel Telemetry configuration", telemetryLogLevel.String())
	}
	if telemetryLogMapping < activeLogLevel {
		activeLogLevel = telemetryLogMapping
	}
	pkgconfig.Set("log_level", logLevelReverseMap[activeLogLevel], pkgconfigmodel.SourceFile)

	// Override config read (if any) with Default values
	pkgconfigsetup.InitConfig(pkgconfig)
	pkgconfigmodel.ApplyOverrideFuncs(pkgconfig)

	ddc, err := getDDExporterConfig(cfg)
	if err == ErrNoDDExporter {
		return pkgconfig, err
	}
	if err != nil {
		return nil, err
	}
	pkgconfig.Set("api_key", string(ddc.API.Key), pkgconfigmodel.SourceFile)
	pkgconfig.Set("site", ddc.API.Site, pkgconfigmodel.SourceFile)

	pkgconfig.Set("dd_url", ddc.Metrics.Endpoint, pkgconfigmodel.SourceFile)

	// Log configs
	pkgconfig.Set("logs_enabled", true, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logs_config.force_use_http", true, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("logs_config.logs_dd_url", ddc.Logs.Endpoint, pkgconfigmodel.SourceFile)
	pkgconfig.Set("logs_config.batch_wait", ddc.Logs.BatchWait, pkgconfigmodel.SourceFile)
	pkgconfig.Set("logs_config.use_compression", ddc.Logs.UseCompression, pkgconfigmodel.SourceFile)
	pkgconfig.Set("logs_config.compression_level", ddc.Logs.CompressionLevel, pkgconfigmodel.SourceFile)

	// APM & OTel trace configs
	pkgconfig.Set("apm_config.enabled", true, pkgconfigmodel.SourceDefault)
	pkgconfig.Set("apm_config.apm_non_local_traffic", true, pkgconfigmodel.SourceDefault)

	pkgconfig.Set("apm_config.debug.port", 0, pkgconfigmodel.SourceDefault)      // Disabled in the otel-agent
	pkgconfig.Set(pkgconfigsetup.OTLPTracePort, 0, pkgconfigmodel.SourceDefault) // Disabled in the otel-agent

	pkgconfig.Set("otlp_config.traces.span_name_as_resource_name", ddc.Traces.SpanNameAsResourceName, pkgconfigmodel.SourceFile)
	pkgconfig.Set("otlp_config.traces.span_name_remappings", ddc.Traces.SpanNameRemappings, pkgconfigmodel.SourceFile)

	pkgconfig.Set("apm_config.receiver_enabled", false, pkgconfigmodel.SourceDefault) // disable HTTP receiver
	pkgconfig.Set("apm_config.ignore_resources", ddc.Traces.IgnoreResources, pkgconfigmodel.SourceFile)
	pkgconfig.Set("apm_config.skip_ssl_validation", ddc.ClientConfig.TLSSetting.InsecureSkipVerify, pkgconfigmodel.SourceFile)
	if v := ddc.Traces.TraceBuffer; v > 0 {
		pkgconfig.Set("apm_config.trace_buffer", v, pkgconfigmodel.SourceFile)
	}
	if addr := ddc.Traces.Endpoint; addr != "" {
		pkgconfig.Set("apm_config.apm_dd_url", addr, pkgconfigmodel.SourceFile)
	}
	if ddc.Traces.ComputeTopLevelBySpanKind {
		pkgconfig.Set("apm_config.features", []string{"enable_otlp_compute_top_level_by_span_kind"}, pkgconfigmodel.SourceFile)
	}

	return pkgconfig, nil
}

func getServiceConfig(cfg *confmap.Conf) (*service.Config, error) {
	var pipelineConfig *service.Config
	s := cfg.Get("service")
	if s == nil {
		return nil, fmt.Errorf("service config not found")
	}
	smap, ok := s.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid service config")
	}
	err := confmap.NewFromStringMap(smap).Unmarshal(&pipelineConfig)
	if err != nil {
		return nil, err
	}
	return pipelineConfig, nil
}

func getDDExporterConfig(cfg *confmap.Conf) (*datadogexporter.Config, error) {
	var configs []*datadogexporter.Config
	var err error
	for k, v := range cfg.ToStringMap() {
		if k != "exporters" {
			continue
		}
		exporters, ok := v.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("invalid exporters config")
		}
		for k, v := range exporters {
			if strings.HasPrefix(k, "datadog") {
				datadogConfig := datadogexporter.CreateDefaultConfig().(*datadogexporter.Config)
				m, ok := v.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("invalid datadog exporter config")
				}
				err = confmap.NewFromStringMap(m).Unmarshal(&datadogConfig)
				if err != nil {
					return nil, err
				}
				configs = append(configs, datadogConfig)
			}
		}
	}
	if len(configs) == 0 {
		return nil, ErrNoDDExporter
	}
	// Check if we have multiple datadog exporters
	// We only support one exporter for now
	// TODO: support multiple exporters
	if len(configs) > 1 {
		return nil, fmt.Errorf("multiple datadog exporters found")
	}

	datadogConfig := configs[0]
	return datadogConfig, nil
}
