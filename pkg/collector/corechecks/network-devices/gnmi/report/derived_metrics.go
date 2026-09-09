// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"errors"

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

	baseTags := buildBaseTags(cfg, snapshot)
	deviceID := buildDeviceID(cfg)
	interfaceInventory := interfaceInventoryByName(deviceID, cfg.Profile, snapshot)
	speedValueMap := portSpeedValueMap(cfg.Profile)

	reportInterfaceBandwidthUsage(s, baseTags, deviceID, snapshot, interfaceInventory, speedValueMap, bandwidthState)
	reportMemoryUsage(s, cfg.Profile, snapshot, baseTags)

	return nil
}

func portSpeedValueMap(profile config.ProfileDefinition) map[string]int {
	for _, metric := range profile.Metrics {
		if normalizeProfilePath(metric.Path) != pathPortSpeed {
			continue
		}
		if len(metric.ValueMap) > 0 {
			return metric.ValueMap
		}
	}
	return nil
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
	speedValueMap map[string]int,
	bandwidthState BandwidthState,
) {
	interfaces := collectInterfaceBandwidthData(baseTags, deviceID, snapshot, inventory, speedValueMap)
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
	speedValueMap map[string]int,
) map[string]*interfaceBandwidthData {
	byPath := indexSnapshotByPath(snapshot)
	interfaces := make(map[string]*interfaceBandwidthData)

	for _, cached := range byPath[pathInOctets] {
		keysID := cacheKeysID(cached.Key.Keys)
		if keysID == "" {
			continue
		}
		value, ok := decodeNumericValue(cached.Entry.Value)
		if !ok {
			continue
		}

		interfaceName := cached.Key.Keys["name"]
		if interfaceName == "" {
			continue
		}

		entry := interfaces[keysID]
		if entry == nil {
			entry = &interfaceBandwidthData{name: interfaceName, inOctets: -1, outOctets: -1}
			interfaces[keysID] = entry
		}
		entry.inOctets = value
		entry.inTags = interfaceBandwidthTags(baseTags, deviceID, interfaceName, inventory)
	}

	for _, cached := range byPath[pathOutOctets] {
		keysID := cacheKeysID(cached.Key.Keys)
		entry := interfaces[keysID]
		if entry == nil {
			continue
		}
		value, ok := decodeNumericValue(cached.Entry.Value)
		if !ok {
			continue
		}
		entry.outOctets = value
		entry.outTags = interfaceBandwidthTags(baseTags, deviceID, entry.name, inventory)
	}

	for _, cached := range byPath[pathPortSpeed] {
		keysID := cacheKeysID(cached.Key.Keys)
		entry := interfaces[keysID]
		if entry == nil {
			continue
		}
		value, ok := decodeMetricValue(cached.Entry.Value, speedValueMap)
		if !ok || value <= 0 {
			continue
		}
		entry.speed = uint64(value)
	}

	return interfaces
}

func interfaceBandwidthTags(baseTags []string, deviceID string, interfaceName string, inventory map[string]devicemetadata.InterfaceMetadata) []string {
	tags := append(ndmutils.CopyStrings(baseTags), "interface:"+interfaceName)
	iface, ok := inventory[interfaceName]
	if !ok || iface.Name == "" {
		return tags
	}

	if iface.Description != "" {
		tags = append(tags, "interface_alias:"+iface.Description)
	}
	return append(tags, internalInterfaceResourceTag(deviceID, iface.Name))
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

func reportMemoryUsage(s sender.Sender, profile config.ProfileDefinition, snapshot []client.CachedValue, baseTags []string) {
	usedMetric, freeMetric, ok := memoryMetricsFromProfile(profile)
	if !ok {
		return
	}

	usedPath := normalizeProfilePath(usedMetric.Path)
	freePath := normalizeProfilePath(freeMetric.Path)
	if _, ok := pathsShareParent(usedPath, freePath); !ok {
		log.Tracef("skip memory usage metric: %s and %s do not share the same parent path", usedPath, freePath)
		return
	}

	byPath := indexSnapshotByPath(snapshot)
	freeByKeys := indexCachedValuesByKeys(byPath[freePath])

	for _, usedCached := range sortCachedValues(byPath[usedPath]) {
		keys := usedCached.Key.Keys
		if len(keys) == 0 {
			continue
		}

		freeCached, ok := freeByKeys[cacheKeysID(keys)]
		if !ok {
			continue
		}

		used, ok := decodeNumericValue(usedCached.Entry.Value)
		if !ok {
			continue
		}
		free, ok := decodeNumericValue(freeCached.Entry.Value)
		if !ok {
			continue
		}

		usage, err := evaluateMemoryUsage(used, used+free)
		if err != nil {
			log.Tracef("skip memory usage metric for %s: %s", cacheKeysID(keys), err)
			continue
		}

		tags := buildMetricTags(baseTags, usedMetric, keys)
		s.Gauge(metricMemoryUsage, usage, "", tags)
	}
}

func memoryMetricsFromProfile(profile config.ProfileDefinition) (used config.MetricConfig, free config.MetricConfig, ok bool) {
	var foundUsed bool
	var foundFree bool
	for _, metric := range profile.Metrics {
		switch metric.Metric {
		case "snmp.memory.used":
			used = metric
			foundUsed = true
		case "snmp.memory.free":
			free = metric
			foundFree = true
		}
	}
	return used, free, foundUsed && foundFree
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
