// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"errors"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	ndmutils "github.com/DataDog/datadog-agent/pkg/networkdevice/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	pathInOctets    = "/openconfig/interfaces/interface/state/counters/in-octets"
	pathOutOctets   = "/openconfig/interfaces/interface/state/counters/out-octets"
	pathPortSpeed   = "/openconfig/interfaces/interface/ethernet/state/port-speed"
	pathMemoryUsed  = "/openconfig/components/component/state/memory/utilized"
	pathMemoryAvail = "/openconfig/components/component/state/memory/available"
)

const (
	metricBandwidthInUsage  = "snmp.ifBandwidthInUsage.rate"
	metricBandwidthOutUsage = "snmp.ifBandwidthOutUsage.rate"
	metricMemoryUsage       = "snmp.memory.usage"
)

// ReportDerivedMetrics emits evaluated NDM metrics such as interface utilization and memory usage.
func ReportDerivedMetrics(s sender.Sender, cfg *config.CheckConfig, snapshot []client.CachedValue, bandwidthState BandwidthState) error {
	if s == nil {
		return errors.New("sender is nil")
	}
	if cfg == nil {
		return errors.New("check config is nil")
	}
	if bandwidthState == nil {
		return errors.New("bandwidth state is nil")
	}

	baseTags := buildBaseTags(cfg)
	deviceID := buildDeviceID(cfg.Instance.Address)
	interfaceInventory := interfaceInventoryByName(deviceID, cfg.Profile.Metadata, snapshot)
	sources := buildDerivedMetricSources(cfg.Profile)

	reportInterfaceBandwidthUsage(s, baseTags, deviceID, snapshot, interfaceInventory, sources, bandwidthState)
	reportMemoryUsage(s, snapshot, baseTags, sources)

	return nil
}

type derivedMetricSources struct {
	inOctets   map[string]config.MetricConfig
	outOctets  map[string]config.MetricConfig
	portSpeed  map[string]config.MetricConfig
	memoryUsed map[string]config.MetricConfig
	memoryFree map[string]config.MetricConfig
}

func buildDerivedMetricSources(profile config.ProfileDefinition) derivedMetricSources {
	sources := derivedMetricSources{
		inOctets:   make(map[string]config.MetricConfig),
		outOctets:  make(map[string]config.MetricConfig),
		portSpeed:  make(map[string]config.MetricConfig),
		memoryUsed: make(map[string]config.MetricConfig),
		memoryFree: make(map[string]config.MetricConfig),
	}
	for _, metric := range profile.Metrics {
		var destination map[string]config.MetricConfig
		switch metric.Metric {
		case "snmp.ifHCInOctets":
			destination = sources.inOctets
		case "snmp.ifHCOutOctets":
			destination = sources.outOctets
		case "snmp.ifInSpeed", "snmp.ifOutSpeed":
			destination = sources.portSpeed
		case "snmp.memory.used":
			destination = sources.memoryUsed
		case "snmp.memory.free":
			destination = sources.memoryFree
		default:
			continue
		}

		for _, path := range snapshotPathAliases(normalizeProfilePath(metric.Path)) {
			destination[path] = metric
		}
	}
	return sources
}

func sourceKeyValue(cached client.CachedValue, metric config.MetricConfig, segment, fallbackKey string) string {
	keyName := metric.SubscriptionKeys()[segment]
	if keyName == "" {
		keyName = fallbackKey
	}
	return cached.Key.Keys[keyName]
}

type interfaceBandwidthData struct {
	name      string
	inOctets  float64
	outOctets float64
	speed     uint64
	inTags    []string
	outTags   []string
}

func reportInterfaceBandwidthUsage(
	s sender.Sender,
	baseTags []string,
	deviceID string,
	snapshot []client.CachedValue,
	inventory map[string]devicemetadata.InterfaceMetadata,
	sources derivedMetricSources,
	bandwidthState BandwidthState,
) {
	interfaces := collectInterfaceBandwidthData(baseTags, deviceID, snapshot, inventory, sources)
	for _, iface := range interfaces {
		if iface.speed == 0 {
			continue
		}
		if iface.inOctets >= 0 && len(iface.inTags) > 0 {
			emitBandwidthUsageRate(s, bandwidthState, iface.name, "ifBandwidthInUsage", iface.speed, iface.inOctets, iface.inTags)
		}
		if iface.outOctets >= 0 && len(iface.outTags) > 0 {
			emitBandwidthUsageRate(s, bandwidthState, iface.name, "ifBandwidthOutUsage", iface.speed, iface.outOctets, iface.outTags)
		}
	}
}

func collectInterfaceBandwidthData(
	baseTags []string,
	deviceID string,
	snapshot []client.CachedValue,
	inventory map[string]devicemetadata.InterfaceMetadata,
	sources derivedMetricSources,
) map[string]*interfaceBandwidthData {
	interfaces := make(map[string]*interfaceBandwidthData)

	for _, cached := range snapshot {
		metric, isInOctets := sources.inOctets[cached.Key.Path]
		if !isInOctets {
			metric, _ = sources.outOctets[cached.Key.Path]
		}
		if metric.Path == "" {
			metric, _ = sources.portSpeed[cached.Key.Path]
		}
		if metric.Path == "" {
			continue
		}

		interfaceName := sourceKeyValue(cached, metric, "interface", "name")
		if interfaceName == "" {
			continue
		}
		entry := interfaces[interfaceName]
		if entry == nil {
			entry = &interfaceBandwidthData{name: interfaceName, inOctets: -1, outOctets: -1}
			interfaces[interfaceName] = entry
		}

		if isInOctets {
			if value, ok := decodeNumericValue(cached.Entry.Value); ok {
				entry.inOctets = value
				entry.inTags = interfaceBandwidthTags(baseTags, deviceID, interfaceName, inventory)
			}
			continue
		}
		if _, ok := sources.outOctets[cached.Key.Path]; ok {
			if value, ok := decodeNumericValue(cached.Entry.Value); ok {
				entry.outOctets = value
				entry.outTags = interfaceBandwidthTags(baseTags, deviceID, interfaceName, inventory)
			}
			continue
		}
		if value, ok := decodeMetricValue(cached.Entry.Value, metric.ValueMap); ok && value > 0 {
			entry.speed = uint64(value)
		}
	}

	return interfaces
}

func interfaceBandwidthTags(baseTags []string, deviceID string, interfaceName string, inventory map[string]devicemetadata.InterfaceMetadata) []string {
	tags := append(ndmutils.CopyStrings(baseTags), "interface:"+interfaceName)
	iface, ok := inventory[interfaceName]
	if !ok || iface.Index <= 0 {
		return tags
	}

	tags = append(tags, "interface_index:"+strconv.Itoa(int(iface.Index)))
	if iface.Description != "" {
		tags = append(tags, "interface_alias:"+iface.Description)
	}
	return append(tags, internalInterfaceResourceTag(deviceID, iface.Index))
}

func emitBandwidthUsageRate(
	s sender.Sender,
	bandwidthState BandwidthState,
	interfaceName string,
	usageName string,
	ifSpeed uint64,
	octets float64,
	tags []string,
) {
	usageValue := ((octets * 8) / float64(ifSpeed)) * 100.0
	interfaceID := interfaceName + "." + usageName
	rate, err := bandwidthState.calculateUsageRate(interfaceID, ifSpeed, usageValue)
	if err != nil {
		log.Tracef("skip bandwidth usage metric %s for %s: %s", usageName, interfaceName, err)
		return
	}

	metricName := "snmp." + usageName + ".rate"
	if usageName == "ifBandwidthInUsage" {
		metricName = metricBandwidthInUsage
	} else if usageName == "ifBandwidthOutUsage" {
		metricName = metricBandwidthOutUsage
	}
	s.Gauge(metricName, rate, "", tags)
}

type memoryComponentData struct {
	used    float64
	free    float64
	tags    []string
	hasUsed bool
	hasFree bool
}

func reportMemoryUsage(s sender.Sender, snapshot []client.CachedValue, baseTags []string, sources derivedMetricSources) {
	components := make(map[string]*memoryComponentData)

	for _, cached := range snapshot {
		metric, isUsed := sources.memoryUsed[cached.Key.Path]
		if !isUsed {
			metric, _ = sources.memoryFree[cached.Key.Path]
		}
		if metric.Path == "" {
			continue
		}
		componentName := sourceKeyValue(cached, metric, "component", "name")
		if componentName == "" {
			continue
		}

		entry := components[componentName]
		if entry == nil {
			entry = &memoryComponentData{tags: append(ndmutils.CopyStrings(baseTags), "memory:"+componentName)}
			components[componentName] = entry
		}

		if isUsed {
			if value, ok := decodeNumericValue(cached.Entry.Value); ok {
				entry.used = value
				entry.hasUsed = true
			}
			continue
		}
		if value, ok := decodeNumericValue(cached.Entry.Value); ok {
			entry.free = value
			entry.hasFree = true
		}
	}

	for componentName, entry := range components {
		if !entry.hasUsed || !entry.hasFree {
			continue
		}
		usage, err := evaluateMemoryUsage(entry.used, entry.used+entry.free)
		if err != nil {
			log.Tracef("skip memory usage metric for %s: %s", componentName, err)
			continue
		}
		s.Gauge(metricMemoryUsage, usage, "", entry.tags)
	}
}

func evaluateMemoryUsage(memoryUsed float64, memoryTotal float64) (float64, error) {
	if memoryTotal == 0 {
		return 0, errors.New("cannot evaluate memory usage, total memory is 0")
	}
	if memoryUsed < 0 {
		return 0, errors.New("cannot evaluate memory usage, memory used is < 0")
	}
	return (memoryUsed / memoryTotal) * 100, nil
}
