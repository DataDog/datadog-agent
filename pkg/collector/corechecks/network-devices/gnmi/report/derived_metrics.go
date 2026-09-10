// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"errors"
	"fmt"

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
	speedValueMap := portSpeedValueMap(cfg.Profile)

	reportInterfaceBandwidthUsage(s, baseTags, deviceID, snapshot, interfaceInventory, speedValueMap, bandwidthState)
	reportMemoryUsage(s, snapshot, baseTags)

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
	interfaces := make(map[string]*interfaceBandwidthData)

	for _, cached := range snapshot {
		interfaceName := cached.Key.Keys["name"]
		if interfaceName == "" {
			continue
		}

		entry := interfaces[interfaceName]
		if entry == nil {
			entry = &interfaceBandwidthData{name: interfaceName, inOctets: -1, outOctets: -1}
			interfaces[interfaceName] = entry
		}

		switch cached.Key.Path {
		case pathInOctets:
			if value, ok := decodeNumericValue(cached.Entry.Value); ok {
				entry.inOctets = value
				entry.inTags = interfaceBandwidthTags(baseTags, deviceID, interfaceName, inventory)
			}
		case pathOutOctets:
			if value, ok := decodeNumericValue(cached.Entry.Value); ok {
				entry.outOctets = value
				entry.outTags = interfaceBandwidthTags(baseTags, deviceID, interfaceName, inventory)
			}
		case pathPortSpeed:
			if value, ok := decodeMetricValue(cached.Entry.Value, speedValueMap); ok && value > 0 {
				entry.speed = uint64(value)
			}
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

	tags = append(tags, "interface_index:"+fmt.Sprintf("%d", iface.Index))
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
	used  float64
	free  float64
	tags  []string
	hasOK bool
}

func reportMemoryUsage(s sender.Sender, snapshot []client.CachedValue, baseTags []string) {
	components := make(map[string]*memoryComponentData)

	for _, cached := range snapshot {
		componentName := cached.Key.Keys["name"]
		if componentName == "" {
			continue
		}

		entry := components[componentName]
		if entry == nil {
			entry = &memoryComponentData{tags: append(baseTags, "memory:"+componentName)}
			components[componentName] = entry
		}

		switch cached.Key.Path {
		case pathMemoryUsed:
			if value, ok := decodeNumericValue(cached.Entry.Value); ok {
				entry.used = value
				entry.hasOK = true
			}
		case pathMemoryAvail:
			if value, ok := decodeNumericValue(cached.Entry.Value); ok {
				entry.free = value
				entry.hasOK = true
			}
		}
	}

	for componentName, entry := range components {
		if !entry.hasOK {
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
