// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package configcheck exposes helpers to fetch config.
package configcheck

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// ErrDataPlaneProxyTLS reports receiver TLS configured while the data plane proxies OTLP signals to the Agent.
var ErrDataPlaneProxyTLS = errors.New("OTLP receiver TLS is not supported when the data plane proxies OTLP signals to the Agent")

var dataPlaneProxySignals = []string{
	coreconfig.DataPlaneExperimentalOTLPProxyLogs,
	coreconfig.DataPlaneExperimentalOTLPProxyMetrics,
	coreconfig.DataPlaneExperimentalOTLPProxyTraces,
}

// confmapKeyDelimiter is the delimiter used for keys in go.opentelemetry.io/collector/confmap
// We hardcode it to avoid the import to the dependency
const confmapKeyDelimiter = "::"

// viperKeyKeyDelimiter is the delimiter for viper keys
const viperKeyDelimiter = "."

func convertToStringConfMap(cfg configmodel.Reader, inmap map[string]interface{}, prefix string, path []string, outmap map[string]interface{}) {
	for k, v := range inmap {
		nextPath := append(slices.Clone(path), k)
		vkey := prefix + strings.Join(nextPath, viperKeyDelimiter)
		ckey := strings.Join(nextPath, confmapKeyDelimiter)
		if m, ok := v.(map[string]interface{}); ok {
			prevSize := len(outmap)
			convertToStringConfMap(cfg, m, prefix, nextPath, outmap)
			// Preserve user-declared section markers (e.g. "grpc:" with nil value in YAML)
			// even when defaults expand them into a map with no user-configured sub-keys.
			if len(outmap) == prevSize && (cfg.HasSection(vkey) || cfg.IsConfigured(vkey)) {
				outmap[ckey] = nil
			}
			continue
		}
		// Keep settings that either:
		// 1. are a section like "otlp_config.receiver:" with a nil value
		// 2. have been explicitly configured by the user (YAML, env var, runtime)
		if cfg.HasSection(vkey) || cfg.IsConfigured(vkey) {
			outmap[ckey] = v
		}
	}
}

func readConfigSection(cfg configmodel.Reader, section string) map[string]interface{} {
	stringMap := map[string]interface{}{}

	val := cfg.Get(section)
	if sectionData, ok := val.(map[string]interface{}); ok {
		// Convert from retrieved section into a scoped confmap separated by "::"
		convertToStringConfMap(cfg, sectionData, section+".", nil, stringMap)
	}
	return stringMap
}

// hasSection checks if a subsection of otlp_config section exists in a given config
func hasSection(cfg configmodel.Reader, section string) bool {
	key := coreconfig.OTLPSection + "." + section
	return cfg.HasSection(key) || cfg.IsConfigured(key)
}

// IsConfigEnabled checks if OTLP ingest is configured in a given config.
func IsConfigEnabled(cfg configmodel.Reader) bool {
	return hasSection(cfg, coreconfig.OTLPReceiverSubSectionKey)
}

// isDataPlaneOTLPEnabled reports whether the data plane owns the configured OTLP receiver endpoints.
func isDataPlaneOTLPEnabled(cfg configmodel.Reader) bool {
	return cfg.GetBool(coreconfig.DataPlaneEnabled) && cfg.GetBool(coreconfig.DataPlaneOTLPEnabled)
}

func enabledDataPlaneProxySignals(cfg configmodel.Reader) []string {
	var keys []string
	for _, key := range dataPlaneProxySignals {
		if cfg.GetBool(key) {
			keys = append(keys, key)
		}
	}
	return keys
}

// IsDataPlaneProxyEnabled reports whether the data plane owns OTLP ingest and proxies at least one signal to the Agent,
// which then relocates its OTLP listeners.
func IsDataPlaneProxyEnabled(cfg configmodel.Reader) bool {
	return isDataPlaneOTLPEnabled(cfg) && len(enabledDataPlaneProxySignals(cfg)) > 0
}

// IsDataPlaneOnlyEnabled reports whether the data plane handles all OTLP ingest, so the Agent must not listen for OTLP.
func IsDataPlaneOnlyEnabled(cfg configmodel.Reader) bool {
	return isDataPlaneOTLPEnabled(cfg) && len(enabledDataPlaneProxySignals(cfg)) == 0
}

// IsAgentPipelineEnabled reports whether OTLP ingest is configured and the Agent runs its own OTLP pipeline.
func IsAgentPipelineEnabled(cfg configmodel.Reader) bool {
	return IsConfigEnabled(cfg) && !IsDataPlaneOnlyEnabled(cfg)
}

// ValidateDataPlaneProxy returns an error wrapping ErrDataPlaneProxyTLS when an enabled receiver protocol has a tls
// key in proxy mode. The key counts even when its value is null or empty.
func ValidateDataPlaneProxy(cfg configmodel.Reader) error {
	if !IsDataPlaneProxyEnabled(cfg) {
		return nil
	}
	receiver := readConfigSection(cfg, coreconfig.OTLPReceiverSection)
	var protocols []string
	for _, protocol := range []string{"grpc", "http"} {
		if hasConfMapKey(receiver, "protocols", protocol, "tls") {
			protocols = append(protocols, coreconfig.OTLPReceiverSection+".protocols."+protocol)
		}
	}
	if len(protocols) == 0 {
		return nil
	}
	return fmt.Errorf("%w: remove tls from %s, or set %s to false",
		ErrDataPlaneProxyTLS, strings.Join(protocols, " and "), strings.Join(enabledDataPlaneProxySignals(cfg), " and "))
}

// IsTraceInternalReceiverEnabled reports whether the trace-agent should listen on the internal OTLP traces port. The
// port stays closed when the proxy configuration is invalid, so OTLP traces cannot bypass the rejection.
func IsTraceInternalReceiverEnabled(cfg configmodel.Reader) bool {
	return IsAgentPipelineEnabled(cfg) && ValidateDataPlaneProxy(cfg) == nil
}

// hasConfMapKey reports whether a flattened confmap has the key made of path, or any key below it.
func hasConfMapKey(m map[string]interface{}, path ...string) bool {
	key := strings.Join(path, confmapKeyDelimiter)
	for k := range m {
		if k == key || strings.HasPrefix(k, key+confmapKeyDelimiter) {
			return true
		}
	}
	return false
}
