// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package converters implements the converters for the host profiler collector.
package converters

import (
	"slices"

	"github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl/receiver"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/go-viper/mapstructure/v2"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter/otlphttpexporter"
)

var (
	// Component IDs
	hostprofilerID      = component.MustNewID("hostprofiler")
	otlpReceiverID      = component.MustNewID("otlp")
	otlpHTTPExporterID  = component.MustNewID("otlphttp")
	infraattributesID   = component.MustNewIDWithName("infraattributes", "default")
	resourcedetectionID = component.MustNewID("resourcedetection")
	ddprofilingID       = component.MustNewIDWithName("ddprofiling", "default")
	hpflareID           = component.MustNewIDWithName("hpflare", "default")

	// Component Types
	infraattributesType   = component.MustNewType("infraattributes")
	resourcedetectionType = component.MustNewType("resourcedetection")
)

// hasProcessorType returns true if the processors list contains a processor of the given type.
func hasProcessorType(processors []component.ID, processorType component.Type) bool {
	return slices.ContainsFunc(processors, func(comp component.ID) bool {
		return comp.Type() == processorType
	})
}

func ensureOtlpHTTPConfig(otlpHTTP component.Config) component.Config {
	otlpHTTPConfig := &otlphttpexporter.Config{}
	if err := mapstructure.Decode(otlphttpexporter.NewFactory().CreateDefaultConfig(), otlpHTTPConfig); err != nil {
		log.Warnf("Failed to decode default otlphttp config: %v", err)
	}

	if otlpHTTP != nil {
		if err := mapstructure.Decode(otlpHTTP, otlpHTTPConfig); err != nil {
			log.Warnf("Failed to decode user otlphttp config, using defaults: %v", err)
		}
	}

	// Ensure dd-api-key header is always present and non-empty
	if key, ok := otlpHTTPConfig.ClientConfig.Headers.Get("dd-api-key"); !ok || len(key) == 0 {
		// TODO: fetch api key
		otlpHTTPConfig.ClientConfig.Headers.Set("dd-api-key", "tbd")
		log.Debug("Added dd-api-key to otlp headers")
	}

	return otlpHTTPConfig
}

func ensureHostProfilerConfig(cfg component.Config) *receiver.Config {
	hostProfilerConfig := &receiver.Config{}
	if err := mapstructure.Decode(cfg, hostProfilerConfig); err != nil {
		log.Warnf("Failed to decode hostprofiler config, using defaults: %v", err)
	}

	for _, endpoint := range hostProfilerConfig.SymbolUploader.SymbolEndpoints {
		if len(endpoint.APIKey) == 0 {
			// TODO: fetch necessary object to get keys
			log.Debugf("Adding agent provided API key to %s", endpoint.Site)
		}

		if len(endpoint.AppKey) == 0 {
			// TODO: fetch necessary object to get keys
			log.Debugf("Adding agent provided App key to %s", endpoint.Site)
		}
	}

	return hostProfilerConfig
}
