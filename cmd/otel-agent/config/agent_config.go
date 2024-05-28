// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package config provides a way to convert the OpenTelemetry Collector configuration to the Datadog Agent configuration.
package config

import (
	"context"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/datadogexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/processor/infraattributesprocessor"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	traceconfig "github.com/DataDog/datadog-agent/pkg/trace/config"
	"github.com/DataDog/opentelemetry-mapping-go/pkg/otlp/attributes"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/confmap/converter/expandconverter"
	"go.opentelemetry.io/collector/confmap/provider/envprovider"
	"go.opentelemetry.io/collector/confmap/provider/fileprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpprovider"
	"go.opentelemetry.io/collector/confmap/provider/httpsprovider"
	"go.opentelemetry.io/collector/confmap/provider/yamlprovider"
	"go.opentelemetry.io/collector/service"
)

// NewConfigComponent creates a new config component from the given URIs
func NewConfigComponent(ctx context.Context, uris []string) (config.Component, *traceconfig.AgentConfig, error) {
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
		return nil, nil, err
	}
	cfg, err := resolver.Resolve(ctx)
	if err != nil {
		return nil, nil, err
	}
	ddc, err := getDDExporterConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	sc, err := getServiceConfig(cfg)
	if err != nil {
		return nil, nil, err
	}
	tcfg, err := newTraceAgentConfig(ddc)
	if err != nil {
		return nil, nil, err
	}
	site := ddc.API.Site
	apiKey := string(ddc.API.Key)
	// Create a new config
	pkgconfig := pkgconfigmodel.NewConfig("OTel", "DD", strings.NewReplacer(".", "_"))
	// Set Default values
	pkgconfigsetup.InitConfig(pkgconfig)
	pkgconfig.Set("api_key", apiKey, pkgconfigmodel.SourceLocalConfigProcess)
	pkgconfig.Set("site", site, pkgconfigmodel.SourceLocalConfigProcess)

	pkgconfig.Set("logs_enabled", true, pkgconfigmodel.SourceLocalConfigProcess)
	pkgconfig.Set("logs_config.use_compression", true, pkgconfigmodel.SourceLocalConfigProcess)
	pkgconfig.Set("log_level", sc.Telemetry.Logs.Level, pkgconfigmodel.SourceLocalConfigProcess)
	pkgconfig.Set("apm_config.enabled", true, pkgconfigmodel.SourceLocalConfigProcess)
	pkgconfig.Set("apm_config.apm_non_local_traffic", true, pkgconfigmodel.SourceLocalConfigProcess)

	if ddc.Traces.ComputeTopLevelBySpanKind {
		pkgconfig.Set("apm_config.features", []string{"enable_otlp_compute_top_level_by_span_kind"}, pkgconfigmodel.SourceLocalConfigProcess)
	}

	pkgconfig.Set("apm_config.trace_buffer", ddc.Traces.TraceBuffer, pkgconfigmodel.SourceLocalConfigProcess)

	// Peer service related configs
	pkgconfig.Set("apm_config.peer_service_aggregation", ddc.Traces.PeerServiceAggregation, pkgconfigmodel.SourceLocalConfigProcess)
	pkgconfig.Set("apm_config.peer_tags_aggregation", ddc.Traces.PeerTagsAggregation, pkgconfigmodel.SourceLocalConfigProcess)
	pkgconfig.Set("apm_config.peer_tags", ddc.Traces.PeerTags, pkgconfigmodel.SourceLocalConfigProcess)
	pkgconfig.Set("apm_config.compute_stats_by_span_kind", ddc.Traces.ComputeStatsBySpanKind, pkgconfigmodel.SourceLocalConfigProcess)

	if isInfraAttributesProcessorEnabled(sc) {
		pkgconfig.Set("otel_collector.remote_tagger", true, pkgconfigmodel.SourceLocalConfigProcess)
	}

	return pkgconfig, tcfg, nil
}

func isInfraAttributesProcessorEnabled(sc *service.Config) bool {
	for _, p := range sc.Pipelines {
		for _, pp := range p.Processors {
			if pp.Name() == infraattributesprocessor.Type.String() {
				return true
			}
		}
	}
	return false
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
				var datadogConfig *datadogexporter.Config
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
		return nil, fmt.Errorf("no datadog exporter found")
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

// TODO: remove this
func newTraceAgentConfig(cfg *datadogexporter.Config) (*traceconfig.AgentConfig, error) {
	acfg := traceconfig.New()
	attrsTranslator, err := attributes.NewTranslator(componenttest.NewNopTelemetrySettings())
	if err != nil {
		return acfg, err
	}
	acfg.OTLPReceiver.AttributesTranslator = attrsTranslator
	acfg.OTLPReceiver.SpanNameRemappings = cfg.Traces.SpanNameRemappings
	acfg.OTLPReceiver.SpanNameAsResourceName = cfg.Traces.SpanNameAsResourceName
	acfg.Endpoints[0].APIKey = string(cfg.API.Key)
	acfg.Ignore["resource"] = cfg.Traces.IgnoreResources
	acfg.ReceiverPort = 0 // disable HTTP receiver
	acfg.SkipSSLValidation = cfg.ClientConfig.TLSSetting.InsecureSkipVerify
	acfg.ComputeStatsBySpanKind = cfg.Traces.ComputeStatsBySpanKind
	acfg.PeerTagsAggregation = cfg.Traces.PeerTagsAggregation
	acfg.PeerTags = cfg.Traces.PeerTags
	if v := cfg.Traces.TraceBuffer; v > 0 {
		acfg.TraceBuffer = v
	}
	if addr := cfg.Traces.Endpoint; addr != "" {
		acfg.Endpoints[0].Host = addr
	} else {
		acfg.Endpoints[0].Host = "https://trace.agent.datadoghq.com"
	}
	if cfg.Traces.ComputeTopLevelBySpanKind {
		acfg.Features["enable_otlp_compute_top_level_by_span_kind"] = struct{}{}
	}
	return acfg, nil
}
