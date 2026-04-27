// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package openmetrics

import "fmt"

func normalizeMetricConfig(cfg *scraperConfig) (map[string]map[string]interface{}, error) {
	config := map[string]map[string]interface{}{}
	options := []struct {
		name    string
		entries []interface{}
	}{
		{name: "metrics", entries: cfg.metrics},
		{name: "extra_metrics", entries: cfg.extraMetrics},
	}

	for _, option := range options {
		for i, entry := range option.entries {
			switch value := entry.(type) {
			case string:
				config[value] = map[string]interface{}{"name": value, "type": transformerNative}
			case map[string]string:
				for rawMetricName, metricName := range value {
					config[rawMetricName] = map[string]interface{}{"name": metricName, "type": transformerNative}
				}
			default:
				entryMap, ok := normalizeMap(value)
				if !ok {
					return nil, fmt.Errorf("entry #%d of setting `%s` must be a string or a mapping", i+1, option.name)
				}
				for rawMetricName, rawConfig := range entryMap {
					switch metricConfig := rawConfig.(type) {
					case string:
						config[rawMetricName] = map[string]interface{}{"name": metricConfig, "type": transformerNative}
					default:
						normalized, ok := normalizeMap(metricConfig)
						if !ok {
							return nil, fmt.Errorf("value of entry `%s` of setting `%s` must be a string or a mapping", rawMetricName, option.name)
						}
						metricData := copyConfigMap(normalized)
						if _, ok := metricData["name"]; !ok {
							metricData["name"] = rawMetricName
						}
						if _, ok := metricData["type"]; !ok {
							metricData["type"] = transformerNative
						}
						config[rawMetricName] = metricData
					}
				}
			}
		}
	}

	return config, nil
}

func normalizeMap(raw interface{}) (map[string]interface{}, bool) {
	switch value := raw.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(value))
		for k, v := range value {
			out[k] = v
		}
		return out, true
	case map[interface{}]interface{}:
		out := make(map[string]interface{}, len(value))
		for k, v := range value {
			out[fmt.Sprint(k)] = v
		}
		return out, true
	case map[string]string:
		out := make(map[string]interface{}, len(value))
		for k, v := range value {
			out[k] = v
		}
		return out, true
	default:
		return nil, false
	}
}

func copyConfigMap(in map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyStringSlice(in []string) []string {
	return append([]string(nil), in...)
}
