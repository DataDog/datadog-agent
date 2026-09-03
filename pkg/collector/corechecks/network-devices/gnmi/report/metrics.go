// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package report translates gNMI cache snapshots into Datadog metrics.
package report

import (
	"errors"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
	ndmutils "github.com/DataDog/datadog-agent/pkg/networkdevice/utils"
)

const defaultDeviceNamespace = "default"

// ReportMetrics submits snmp.* metrics from a client snapshot using profile mappings.
func ReportMetrics(s sender.Sender, cfg *config.CheckConfig, snapshot []client.CachedValue) error {
	if s == nil {
		return errors.New("sender is nil")
	}
	if cfg == nil {
		return errors.New("check config is nil")
	}

	baseTags := buildBaseTags(cfg)
	byPath := indexSnapshotByPath(snapshot)

	for _, metric := range cfg.Profile.Metrics {
		values := byPath[normalizeProfilePath(metric.Path)]
		for _, cached := range values {
			tags := buildMetricTags(baseTags, metric, cached.Key.Keys)
			value, ok := decodeMetricValue(cached.Entry.Value, metric.ValueMap)
			if !ok {
				continue
			}
			submitMetric(s, metric, value, tags)
		}
	}

	return nil
}

func indexSnapshotByPath(snapshot []client.CachedValue) map[string][]client.CachedValue {
	byPath := make(map[string][]client.CachedValue, len(snapshot))
	for _, cached := range snapshot {
		byPath[cached.Key.Path] = append(byPath[cached.Key.Path], cached)
	}
	return byPath
}

func normalizeProfilePath(path string) string {
	trimmed := path
	if trimmed == "" {
		return trimmed
	}
	if trimmed[0] != '/' {
		return "/" + trimmed
	}
	return trimmed
}

func buildBaseTags(cfg *config.CheckConfig) []string {
	address := cfg.Instance.Address
	deviceID := buildDeviceID(address)
	tags := []string{
		"device_ip:" + address,
		"device_id:" + deviceID,
	}
	return append(tags, ndmutils.CopyStrings(cfg.Instance.Tags)...)
}

func buildDeviceID(address string) string {
	return defaultDeviceNamespace + ":" + address
}

func buildMetricTags(baseTags []string, metric config.MetricConfig, keys map[string]string) []string {
	tags := ndmutils.CopyStrings(baseTags)
	for segment, keyName := range metric.Tags {
		if keyName == "" {
			continue
		}
		value, ok := keys[keyName]
		if !ok || value == "" {
			continue
		}
		tags = append(tags, segment+":"+value)
	}
	return tags
}

func decodeMetricValue(value any, valueMap map[string]int) (float64, bool) {
	if len(valueMap) > 0 {
		if mapped, ok := decodeMappedValue(value, valueMap); ok {
			return mapped, true
		}
	}
	return decodeNumericValue(value)
}

func decodeMappedValue(value any, valueMap map[string]int) (float64, bool) {
	strValue, ok := value.(string)
	if !ok {
		return 0, false
	}
	mapped, ok := valueMap[strValue]
	if !ok {
		return 0, false
	}
	return float64(mapped), true
}

func decodeNumericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int8:
		return float64(typed), true
	case int16:
		return float64(typed), true
	case int32:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case uint:
		return float64(typed), true
	case uint8:
		return float64(typed), true
	case uint16:
		return float64(typed), true
	case uint32:
		return float64(typed), true
	case uint64:
		return float64(typed), true
	case float32:
		return float64(typed), true
	case float64:
		return typed, true
	case bool:
		if typed {
			return 1, true
		}
		return 0, true
	case string:
		parsed, err := strconv.ParseFloat(typed, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func submitMetric(s sender.Sender, metric config.MetricConfig, value float64, tags []string) {
	switch metric.Type {
	case config.MetricTypeGauge:
		s.Gauge(metric.Metric, value, "", tags)
	case config.MetricTypeMonotonicCount:
		s.MonotonicCount(metric.Metric, value, "", tags)
	}
}
