// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package converterimpl provides the implementation of the otel-agent converter.
package converterimpl

import (
	"go.opentelemetry.io/collector/confmap"
)

func changeDefaultConfigsForDatadogConnector(conf *confmap.Conf) {
	stringMapConf := conf.ToStringMap()
	connectors, ok := stringMapConf["connectors"]
	if !ok {
		return
	}
	connectorMap, ok := connectors.(map[string]any)
	if !ok {
		return
	}
	changed := false
	for name, ccfg := range connectorMap {
		if componentName(name) != "datadog" {
			continue
		}
		var ddconnectorCfg map[string]any
		if ccfg == nil {
			ddconnectorCfg = map[string]any{"span_name_as_resource_name": true}
			connectorMap[name] = ddconnectorCfg
			changed = true
		} else {
			ddconnectorCfg, ok = ccfg.(map[string]any)
			if !ok {
				continue
			}
			_, ok = ddconnectorCfg["span_name_as_resource_name"]
			if !ok {
				ddconnectorCfg["span_name_as_resource_name"] = true
				changed = true
			}
		}
	}
	if changed {
		*conf = *confmap.NewFromStringMap(stringMapConf)
	}
}
