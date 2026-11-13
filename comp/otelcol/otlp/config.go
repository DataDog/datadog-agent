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
	"github.com/spf13/cast"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/exporter/serializerexporter"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/configcheck"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	tagutil "github.com/DataDog/datadog-agent/pkg/util/tags"
)

// errors for ADP OTLP proxy configuration validation
var (
	ErrAgentReceiverEndpointNotConfigured = errors.New("data plane OTLP enabled but Agent receiver endpoint is not configured")
	ErrAgentReceiverEndpointCollision     = errors.New("relocated Agent OTLP receiver endpoint conflicts with another listener")
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

	otlpReceiverConfig := configcheck.ReadConfigSection(cfg, coreconfig.OTLPReceiverSection)
	otlpReceiverConfigMap := otlpReceiverConfig.ToStringMap()

	if configcheck.IsDataPlaneProxyEnabled(cfg) {
		if err := configcheck.ValidateDataPlaneProxy(cfg); err != nil {
			errs = append(errs, err)
		} else {
			errs = append(errs, relocateReceiverListeners(cfg, otlpReceiverConfigMap))
		}
	}

	tracePort, err := portToUint(cfg.GetInt(coreconfig.OTLPTracePort))
	if err != nil {
		errs = append(errs, fmt.Errorf("internal trace port is invalid: %w", err))
	}
	metricsEnabled := cfg.GetBool(coreconfig.OTLPMetricsEnabled)
	tracesEnabled := cfg.GetBool(coreconfig.OTLPTracesEnabled)
	logsEnabled := cfg.GetBool(coreconfig.OTLPLogsEnabled)
	TracesInfraAttributesEnabled := cfg.GetBool(coreconfig.OTLPTracesInfraAttrEnabled)
	tracesContainerTagPromotion := cfg.GetString(coreconfig.OTLPTracesInfraAttrContainerTagPromotion)
	logsTagsAsDDTags := cfg.GetBool(coreconfig.OTLPLogsInfraAttrTagsAsDDTags)
	metricsInfraAttrsAsTags := cfg.GetBool(coreconfig.OTLPMetricsInfraAttrAsTags)

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

	// resource_attributes_as_tags promotes every resource attribute onto metrics as a
	// tag, which already surfaces the custom tagger-derived tags that
	// infra_attributes.as_tags targets -- making the two options redundant. Worse,
	// enabling both leaks the internal `datadog.container.tag.` namespace as literal
	// metric tags (the promoted prefixed copy gets dumped verbatim). They are therefore
	// mutually exclusive: resource_attributes_as_tags wins and promotion is disabled.
	if metricsInfraAttrsAsTags && cast.ToBool(metricsConfigMap["resource_attributes_as_tags"]) {
		log.Warn("otlp_config.metrics.infra_attributes.as_tags is ignored because otlp_config.metrics.resource_attributes_as_tags is enabled: the latter already promotes custom tags onto metrics")
		metricsInfraAttrsAsTags = false
	}
	mc, err := normalizeMetricsConfig(metricsConfigMap, false)
	if err != nil {
		errs = append(errs, fmt.Errorf("failed to normalize metrics config: %w", err))
	}

	debugConfig := configcheck.ReadConfigSection(cfg, coreconfig.OTLPDebug)
	debugMap := debugConfig.ToStringMap()
	// If the user explicitly declares the otlp_config.debug section but does not set a
	// verbosity, attach the debug exporter using the default verbosity. When the section
	// is absent entirely, verbosity is left unset so the debug exporter is not attached.
	if _, ok := debugMap["verbosity"]; !ok {
		if cfg.HasSection(coreconfig.OTLPDebug) || cfg.IsConfigured(coreconfig.OTLPDebug) {
			debugMap["verbosity"] = cfg.GetString(coreconfig.OTLPDebug + ".verbosity")
		}
	}

	return PipelineConfig{
		OTLPReceiverConfig:           otlpReceiverConfigMap,
		TracePort:                    tracePort,
		MetricsEnabled:               metricsEnabled,
		TracesEnabled:                tracesEnabled,
		LogsEnabled:                  logsEnabled,
		Metrics:                      mc,
		TracesInfraAttributesEnabled: TracesInfraAttributesEnabled,
		TracesContainerTagPromotion:  tracesContainerTagPromotion,
		LogsTagsAsDDTags:             logsTagsAsDDTags,
		MetricsInfraAttrsAsTags:      metricsInfraAttrsAsTags,
		MetricsBatch:                 metricsBatchConfig.ToStringMap(),
		Logs:                         logsConfig.ToStringMap(),
		Debug:                        debugMap,
	}, multierr.Combine(errs...)
}

// relocateReceiverListeners moves enabled Agent OTLP listeners off the endpoints configured in otlp_config.receiver,
// which the data plane owns in proxy mode. All validation errors are reported together, and nothing is changed unless
// validation passes. Only the receiver copy is modified.
func relocateReceiverListeners(cfg config.Reader, receiver map[string]interface{}) error {
	protocols, _ := receiver["protocols"].(map[string]interface{})
	listeners := []struct {
		protocol           string
		key                string
		config             map[string]interface{}
		configuredEndpoint string
		agentEndpoint      string
	}{
		{protocol: "grpc", key: coreconfig.DataPlaneExperimentalOTLPProxyAgentReceiverGRPCEndpoint},
		{protocol: "http", key: coreconfig.DataPlaneExperimentalOTLPProxyAgentReceiverHTTPEndpoint},
	}

	// Listeners that fail validation keep a nil config, which excludes them from collision checks and relocation.
	var errs []error
	for i := range listeners {
		listener := &listeners[i]
		section, enabled := protocols[listener.protocol]
		if !enabled {
			if cfg.IsConfigured(listener.key) {
				log.Warnf("%s is ignored because the OTLP/%s receiver is not enabled", listener.key, listener.protocol)
			}
			continue
		}

		listenerConfig := map[string]interface{}{}
		if section != nil {
			var ok bool
			listenerConfig, ok = section.(map[string]interface{})
			if !ok {
				errs = append(errs, fmt.Errorf("OTLP/%s receiver configuration must be a map", listener.protocol))
				continue
			}
		}

		agentEndpoint := cfg.GetString(listener.key)
		if agentEndpoint == "" {
			errs = append(errs, fmt.Errorf("%w: %s", ErrAgentReceiverEndpointNotConfigured, listener.key))
			continue
		}
		transport, _ := listenerConfig["transport"].(string)
		if isUnixTransport(transport) && !cfg.IsConfigured(listener.key) {
			errs = append(errs, fmt.Errorf("%w: %s must be explicitly configured with a Unix socket path when OTLP/%s transport is %q",
				ErrAgentReceiverEndpointNotConfigured, listener.key, listener.protocol, transport))
			continue
		}
		configuredEndpoint, explicit := listenerConfig["endpoint"].(string)
		if !explicit {
			configuredEndpoint = cfg.GetString(coreconfig.OTLPReceiverSection + ".protocols." + listener.protocol + ".endpoint")
		}

		listener.config = listenerConfig
		listener.agentEndpoint = agentEndpoint
		listener.configuredEndpoint = configuredEndpoint
	}

	for i, listener := range listeners {
		if listener.config == nil {
			continue
		}
		for j, other := range listeners {
			if other.config == nil {
				continue
			}
			// i < j reports a collision between the two relocated endpoints once, not from both sides.
			if listener.agentEndpoint == other.configuredEndpoint || (i < j && listener.agentEndpoint == other.agentEndpoint) {
				errs = append(errs, fmt.Errorf("%w: %s = %q conflicts with OTLP/%s", ErrAgentReceiverEndpointCollision, listener.key, listener.agentEndpoint, other.protocol))
			}
		}
	}

	if len(errs) > 0 {
		return multierr.Combine(errs...)
	}

	for _, listener := range listeners {
		if listener.config == nil {
			continue
		}
		listener.config["endpoint"] = listener.agentEndpoint
		protocols[listener.protocol] = listener.config
		log.Infof("Relocating Agent OTLP/%s receiver from %q to %q", listener.protocol, listener.configuredEndpoint, listener.agentEndpoint)
	}
	return nil
}

func isUnixTransport(transport string) bool {
	switch transport {
	case "unix", "unixgram", "unixpacket":
		return true
	}
	return false
}

func normalizeMetricsConfig(metricsConfigMap map[string]interface{}, strict bool) (map[string]interface{}, error) {
	// metricsConfigMap doesn't strictly match the types present in MetricsConfig struct
	// so to get properly type map we need to decode it twice

	// We need to start with default config to get the corrent default values
	cf := serializerexporter.NewFactoryForAgent(nil, nil, nil, serializerexporter.TelemetryStore{}).CreateDefaultConfig()

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
