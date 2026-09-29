// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package accesspoint

import (
	"errors"
	"math"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/overlay"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// Metrics evaluates current declarations and carries forward the endpoint of
// the most recent earlier declaration. It has no mutable tick state: omitted
// APs stay present and repeated evaluation cannot compound prior jitter.
func (m *Model) Metrics(phaseIndex int, elapsed time.Duration, ordinal int64, at time.Time) ([]*metrics.Serie, error) {
	if phaseIndex < 0 || phaseIndex >= len(m.scenario.Phases) || elapsed < 0 || elapsed > m.scenario.Phases[phaseIndex].Duration.Duration || ordinal < 0 {
		return nil, errors.New("invalid AP metric phase, elapsed time, or sample ordinal")
	}
	var result []*metrics.Serie
	for apOrdinal, ap := range m.accessPoints {
		deviceTags := append(slices.Clone(ap.device.Tags), metadata.DeviceResourceTag(ap.device.ID))
		for ifaceIndex := -1; ifaceIndex < len(ap.definition.Interfaces); ifaceIndex++ {
			ifaceName := ""
			tags := slices.Clone(deviceTags)
			if ifaceIndex >= 0 {
				iface := ap.definition.Interfaces[ifaceIndex]
				ifaceName = iface.Name
				tags = append(tags, ap.interfaces[ifaceIndex].IDTags...)
				tags = append(tags, metadata.InterfaceResourceTag(ap.device.ID, iface.Index))
				if iface.Kind == "radio" {
					for _, wireless := range ap.wireless {
						if wireless.InterfaceByIntegrationID == metadata.InterfaceID(ap.device.ID, iface.Index) {
							tags = append(tags, "bssid:"+wireless.BSSID, "ssid:"+wireless.SSID, "band:"+wireless.Band)
						}
					}
				}
			}
			names := map[string]bool{}
			for i := 0; i <= phaseIndex; i++ {
				for name := range m.patterns(i, ap.definition.Name, ifaceName) {
					names[name] = true
				}
			}
			ordered := make([]string, 0, len(names))
			for name := range names {
				ordered = append(ordered, name)
			}
			slices.Sort(ordered)
			for _, name := range ordered {
				var pattern schema.Pattern
				definedPhase := phaseIndex
				for ; definedPhase >= 0; definedPhase-- {
					value, exists := m.patterns(definedPhase, ap.definition.Name, ifaceName)[name]
					if exists {
						pattern = value
						break
					}
				}
				effectiveElapsed := elapsed
				if definedPhase < phaseIndex {
					effectiveElapsed = m.scenario.Phases[definedPhase].Duration.Duration
				}
				variance := 0.03
				if statusMetric(name) {
					variance = 0
				}
				ctx := overlay.Context{Scenario: m.scenario, Group: schema.GroupDef{Group: "access-point:" + ap.definition.Name, BaselineVariance: variance}, Seed: m.seed, DeviceOrdinal: apOrdinal, PhaseIndex: definedPhase, Elapsed: effectiveElapsed, Stream: schema.Metrics, SampleOrdinal: ordinal}
				value, err := overlay.PatternValue(ctx, pattern, "ap:"+ap.definition.Name+"/"+ifaceName+"/"+name)
				if err != nil {
					return nil, err
				}
				value = clampMetric(name, value)
				mtype := metricType(name)
				if mtype == metrics.APICountType {
					value = math.Round(value)
				}
				result = append(result, &metrics.Serie{Name: name, Host: ap.device.Name, Tags: tagset.CompositeTagsFromSlice(slices.Clone(tags)), MType: mtype, Source: metrics.MetricSourceSnmp, Interval: int64(MetricsCadence / time.Second), Points: []metrics.Point{{Ts: float64(at.UnixNano()) / 1e9, Value: value}}})
			}
		}
	}
	return result, nil
}

func (m *Model) patterns(phase int, ap, iface string) map[string]schema.Pattern {
	values := m.scenario.Phases[phase].NetworkMetrics[ap]
	if iface == "" {
		return values.Device
	}
	return values.Interfaces[iface]
}

func statusMetric(name string) bool {
	return name == "snmp.device.reachable" || name == "snmp.ifAdminStatus" || name == "snmp.ifOperStatus"
}

func metricType(name string) metrics.APIMetricType {
	switch name {
	case "snmp.ifHCInOctets", "snmp.ifHCOutOctets", "snmp.ifInErrors", "snmp.ifOutErrors", "snmp.ifInDiscards", "snmp.ifOutDiscards", "snmp.apBSSTxBytes", "snmp.apBSSRxBytes":
		return metrics.APICountType
	default:
		return metrics.APIGaugeType
	}
}

func clampMetric(name string, value float64) float64 {
	switch name {
	case "snmp.apChannelNoise":
		return max(-120, min(0, value))
	case "snmp.cpu.usage", "snmp.memory.usage", "snmp.apChannelBwRate", "snmp.apChannelFrameRetryRate", "snmp.apChannelFrameReceiveErrorRate", "snmp.apBSSBwRate", "snmp.ifBandwidthInUsage.rate", "snmp.ifBandwidthOutUsage.rate":
		return max(0, min(100, value))
	default:
		return max(0, value)
	}
}
