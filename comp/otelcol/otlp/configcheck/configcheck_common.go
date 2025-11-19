// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2021-present Datadog, Inc.

// Package configcheck exposes helpers to fetch config.
package configcheck

import (
	"slices"
	"strings"

	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/config/viperconfig"
)

// confmapKeyDelimiter is the delimiter used for keys in go.opentelemetry.io/collector/confmap
// We hardcode it to avoid the import to the dependency
const confmapKeyDelimiter = "::"
const viperKeyDelimiter = "."

func convertToStringConfMap(cfg configmodel.Reader, inmap map[string]interface{}, prefix string, path []string, outmap map[string]interface{}) {
	for k, v := range inmap {
		nextPath := append(slices.Clone(path), k)
		vkey := prefix + strings.Join(nextPath, viperKeyDelimiter)
		ckey := strings.Join(nextPath, confmapKeyDelimiter)
		if m, ok := v.(map[string]interface{}); ok {
			convertToStringConfMap(cfg, m, prefix, nextPath, outmap)
			continue
		}
		// Keep settings that either:
		// 1. are a section like "otlp_config.receiver:" with a nil value
		// 2. have a scalar instead of a section like "otlp_config.receiver: 1234"
		// 3. have a non-nil value (such as settings with defined default values)
		if cfg.HasSection(vkey) || cfg.IsConfigured(vkey) || cfg.Get(vkey) != nil {
			outmap[ckey] = v
		}
	}
}

func readConfigSection(cfg configmodel.Reader, section string) map[string]interface{} {
	stringMap := map[string]interface{}{}

	// Get all layers combined when using viper, which doesn't correctly
	// merge all layers when calling .Get(key)
	val := viperconfig.GetViperCombine(cfg, section)
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

// IsConfigEnabled checks if OTLP pipeline is enabled in a given config.
func IsConfigEnabled(cfg configmodel.Reader) bool {
	return hasSection(cfg, coreconfig.OTLPReceiverSubSectionKey)
}
