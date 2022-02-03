// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

package config

import (
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// OTLP configuration paths.
const (
	OTLPSection               = "otlp_config"
	OTLPTracesSubSectionKey   = "traces"
	OTLPTracePort             = OTLPSection + "." + OTLPTracesSubSectionKey + ".internal_port"
	OTLPTracesEnabled         = OTLPSection + "." + OTLPTracesSubSectionKey + ".enabled"
	OTLPReceiverSubSectionKey = "receiver"
	OTLPReceiverSection       = OTLPSection + "." + OTLPReceiverSubSectionKey
	OTLPMetricsSubSectionKey  = "metrics"
	OTLPMetrics               = OTLPSection + "." + OTLPMetricsSubSectionKey
	OTLPMetricsEnabled        = OTLPSection + "." + OTLPMetricsSubSectionKey + ".enabled"
	OTLPTagCardinalityKey     = OTLPMetrics + ".tag_cardinality"
)

// SetupOTLP related configuration.
func SetupOTLP(config Config) {
	config.BindEnvAndSetDefault(OTLPTracePort, 5003)
	config.BindEnvAndSetDefault(OTLPMetricsEnabled, true)
	config.BindEnvAndSetDefault(OTLPTracesEnabled, true)

	// NOTE: This only partially works.
	// The environment variable is also manually checked in pkg/otlp/config.go
	config.BindEnvAndSetDefault(OTLPTagCardinalityKey, "low", "DD_OTLP_TAG_CARDINALITY")

	config.SetKnown(OTLPMetrics)
	// Set all subkeys of otlp.metrics as known
	config.SetKnown(OTLPMetrics + ".*")
	config.SetKnown(OTLPReceiverSection)
	// Set all subkeys of otlp.receiver as known
	config.SetKnown(OTLPReceiverSection + ".*")
}

// promoteExperimentalOTLP checks if "experimental.otlp" is set and promotes it to the top level
// "otlp" configuration if unset.
//
// TODO(gbbr): This is to keep backwards compatibility while we've gone public beta and should
// be completely removed once 7.35.0 is out.
func promoteExperimentalOTLP(cfg Config) {
	if !cfg.IsSet("experimental.otlp") {
		return
	}
	log.Warn(`OpenTelemetry OTLP receiver configuration is now public beta and has been moved out of the "experimental" section. ` +
		`This section will be deprecated in a future Datadog Agent release. Please use the same configuration as part of the top level "otlp_config" section instead.`)
	if k := "experimental.otlp.metrics"; cfg.IsSet(k) {
		for key, val := range cfg.GetStringMap(k) {
			cfg.Set(OTLPMetrics+"."+key, val)
		}
	}
	if k := "experimental.otlp.metrics_enabled"; cfg.IsSet(k) {
		cfg.Set(OTLPMetricsEnabled, cfg.GetBool(k))
	}
	if k := "experimental.otlp.tag_cardinality"; cfg.IsSet(k) {
		cfg.Set(OTLPTagCardinalityKey, cfg.GetString(k))
	}
	if k := "experimental.otlp.traces_enabled"; cfg.IsSet(k) {
		cfg.Set(OTLPTracesEnabled, cfg.GetBool(k))
	}
	if v := cfg.GetString("experimental.otlp.internal_traces_port"); v != "" {
		cfg.Set(OTLPTracePort, v)
	}
	if v, ok := cfg.GetStringMap("experimental.otlp")[OTLPReceiverSubSectionKey]; ok {
		if v == nil {
			cfg.Set(OTLPReceiverSection, nil)
		} else {
			for key, val := range cfg.GetStringMap("experimental.otlp.receiver") {
				cfg.Set(OTLPReceiverSection+"."+key, val)
			}
		}
	}
	if v := cfg.GetString("experimental.otlp.http_port"); v != "" {
		cfg.Set(OTLPReceiverSection+".protocols.http.endpoint", net.JoinHostPort(getBindHost(cfg), v))
	}
	if v := cfg.GetString("experimental.otlp.grpc_port"); v != "" {
		cfg.Set(OTLPReceiverSection+".protocols.grpc.endpoint", net.JoinHostPort(getBindHost(cfg), v))
	}
}
