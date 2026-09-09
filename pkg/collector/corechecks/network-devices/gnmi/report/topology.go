// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"net"
	"sort"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/client"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/gnmi/config"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/lldp"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

const topologyLinkSourceTypeLLDP = "lldp"

func buildTopologyLinks(deviceID string, topology config.TopologyConfig, snapshot []client.CachedValue, interfaces []devicemetadata.InterfaceMetadata) []devicemetadata.TopologyLinkMetadata {
	if topology.IsZero() {
		return nil
	}

	index := indexSnapshot(snapshot)
	neighborKeys := lldpNeighborKeys(index, topology)
	if len(neighborKeys) == 0 {
		return nil
	}

	sort.Slice(neighborKeys, func(i, j int) bool {
		interfaceKey := topology.NeighborInterfaceKey()
		neighborIDKey := topology.NeighborIDKey()
		left := neighborKeys[i][interfaceKey] + "\x00" + neighborKeys[i][neighborIDKey]
		right := neighborKeys[j][interfaceKey] + "\x00" + neighborKeys[j][neighborIDKey]
		return left < right
	})

	links := make([]devicemetadata.TopologyLinkMetadata, 0, len(neighborKeys))

	for _, keys := range neighborKeys {
		interfaceName := keys[topology.NeighborInterfaceKey()]
		neighborKey := keys[topology.NeighborIDKey()]

		remoteChassisIDType := normalizeLLDPIDType(firstStringValue(index, topology.LLDP.ChassisIDType, keys))
		remoteChassisID := formatLLDPID(remoteChassisIDType, firstStringValue(index, topology.LLDP.ChassisID, keys))

		remotePortIDType := normalizeLLDPIDType(firstStringValue(index, topology.LLDP.PortIDType, keys))
		remotePortID := formatLLDPID(remotePortIDType, firstStringValue(index, topology.LLDP.PortID, keys))

		remoteName := firstStringValue(index, topology.LLDP.SystemName, keys)
		remoteMgmtAddress := firstStringValue(index, topology.LLDP.ManagementAddress, keys)
		remoteMgmtIP := canonicalManagementIPAddress(remoteMgmtAddress)
		remoteDevice := &devicemetadata.TopologyLinkDevice{
			Name:        remoteName,
			Description: firstStringValue(index, topology.LLDP.SystemDescription, keys),
			ID:          remoteChassisID,
			IDType:      remoteChassisIDType,
			IPAddress:   remoteTopologyIPAddress(remoteMgmtIP, remoteMgmtAddress),
		}
		if remoteMgmtIP != "" {
			remoteDevice.DDID = buildDeviceIDFromConfigAddress(remoteMgmtIP)
		}

		linkID := buildTopologyLinkID(deviceID, interfaceName, remoteChassisIDType, remoteChassisID, neighborKey)
		links = append(links, devicemetadata.TopologyLinkMetadata{
			ID:          linkID,
			SourceType:  topologyLinkSourceTypeLLDP,
			Integration: string(integrations.Gnmi),
			Remote: &devicemetadata.TopologyLinkSide{
				Device: remoteDevice,
				Interface: &devicemetadata.TopologyLinkInterface{
					ID:          remotePortID,
					IDType:      remotePortIDType,
					Description: firstStringValue(index, topology.LLDP.PortDescription, keys),
				},
			},
			Local: &devicemetadata.TopologyLinkSide{
				Interface: &devicemetadata.TopologyLinkInterface{
					DDID:   buildInterfaceID(deviceID, interfaceName),
					ID:     interfaceName,
					IDType: devicemetadata.IDTypeInterfaceName,
				},
				Device: &devicemetadata.TopologyLinkDevice{
					DDID: deviceID,
				},
			},
		})
	}

	return links
}

// buildTopologyLinkID builds a stable link identifier from the local interface and
// normalized remote chassis ID. The neighbor list key is only used as a fallback
// when chassis-id telemetry is missing, and is normalized with the same rules.
func buildTopologyLinkID(deviceID, interfaceName, remoteChassisIDType, remoteChassisID, neighborKey string) string {
	neighborToken := remoteChassisID
	if neighborToken == "" {
		neighborToken = formatLLDPID(remoteChassisIDType, neighborKey)
	}
	return deviceID + ":" + interfaceName + "." + neighborToken
}

// canonicalManagementIPAddress returns a normalized IP address when the LLDP management
// address is a parseable IP. Hostnames and other non-IP values return empty string.
// This matches the SNMP check, which only extracts IPv4 management addresses from the
// LLDP MIB and never sets remote dd_id from hostnames.
func canonicalManagementIPAddress(address string) string {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return ""
	}
	ip := net.ParseIP(trimmed)
	if ip == nil {
		return ""
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String()
	}
	return ip.String()
}

func remoteTopologyIPAddress(canonicalIP, rawAddress string) string {
	if canonicalIP != "" {
		return canonicalIP
	}
	return strings.TrimSpace(rawAddress)
}

func normalizeLLDPIDType(value string) string {
	normalized := strings.TrimSpace(value)
	if normalized == "" {
		return ""
	}
	if idx := strings.LastIndex(normalized, ":"); idx >= 0 {
		normalized = normalized[idx+1:]
	}
	if mapped, ok := lldp.ChassisIDSubtypeMap[normalized]; ok {
		return mapped
	}
	if mapped, ok := lldp.PortIDSubTypeMap[normalized]; ok {
		return mapped
	}
	return strings.ToLower(normalized)
}

func formatLLDPID(idType, idValue string) string {
	if idValue == "" {
		return ""
	}
	if idType == devicemetadata.IDTypeMacAddress || strings.Contains(idValue, ":") {
		return strings.ToLower(idValue)
	}
	return idValue
}
