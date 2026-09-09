// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package report

import (
	"sort"
	"strconv"
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

	interfaceIndexByIDType := buildInterfaceIndexByIDType(interfaces)
	links := make([]devicemetadata.TopologyLinkMetadata, 0, len(neighborKeys))

	for _, keys := range neighborKeys {
		interfaceName := keys[topology.NeighborInterfaceKey()]
		neighborID := keys[topology.NeighborIDKey()]

		remoteChassisIDType := normalizeLLDPIDType(firstStringValue(index, topology.LLDP.ChassisIDType, keys))
		remoteChassisID := formatLLDPID(remoteChassisIDType, firstStringValue(index, topology.LLDP.ChassisID, keys))

		remotePortIDType := normalizeLLDPIDType(firstStringValue(index, topology.LLDP.PortIDType, keys))
		remotePortID := formatLLDPID(remotePortIDType, firstStringValue(index, topology.LLDP.PortID, keys))

		localInterfaceID := resolveLocalInterface(deviceID, interfaceIndexByIDType, devicemetadata.IDTypeInterfaceName, interfaceName)

		linkID := deviceID + ":" + interfaceName + "." + neighborID
		links = append(links, devicemetadata.TopologyLinkMetadata{
			ID:          linkID,
			SourceType:  topologyLinkSourceTypeLLDP,
			Integration: string(integrations.Gnmi),
			Remote: &devicemetadata.TopologyLinkSide{
				Device: &devicemetadata.TopologyLinkDevice{
					Name:        firstStringValue(index, topology.LLDP.SystemName, keys),
					Description: firstStringValue(index, topology.LLDP.SystemDescription, keys),
					ID:          remoteChassisID,
					IDType:      remoteChassisIDType,
					IPAddress:   firstStringValue(index, topology.LLDP.ManagementAddress, keys),
				},
				Interface: &devicemetadata.TopologyLinkInterface{
					ID:          remotePortID,
					IDType:      remotePortIDType,
					Description: firstStringValue(index, topology.LLDP.PortDescription, keys),
				},
			},
			Local: &devicemetadata.TopologyLinkSide{
				Interface: &devicemetadata.TopologyLinkInterface{
					DDID:   localInterfaceID,
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

type interfaceCandidate struct {
	ifIndex    int32
	isPhysical bool
	macAddress string
}

func resolveLocalInterface(deviceID string, interfaceIndexByIDType map[string]map[string][]interfaceCandidate, localInterfaceIDType string, localInterfaceID string) string {
	if localInterfaceID == "" {
		return ""
	}

	typesToTry := []string{localInterfaceIDType}
	if localInterfaceIDType == "" {
		typesToTry = []string{"mac_address", "interface_name", "interface_alias", "interface_index"}
	}

	matchedCandidates := make(map[int32]interfaceCandidate)
	for _, idType := range typesToTry {
		interfaceIndexByIDValue, ok := interfaceIndexByIDType[idType]
		if !ok {
			continue
		}
		for _, candidate := range interfaceIndexByIDValue[localInterfaceID] {
			matchedCandidates[candidate.ifIndex] = candidate
		}
	}

	if len(matchedCandidates) == 1 {
		for ifIndex := range matchedCandidates {
			return deviceID + ":" + formatInt32(ifIndex)
		}
	}

	if len(matchedCandidates) > 1 {
		if physical, ok := singlePhysicalCandidateSharingMAC(matchedCandidates); ok {
			return deviceID + ":" + formatInt32(physical.ifIndex)
		}
	}

	return ""
}

func singlePhysicalCandidateSharingMAC(candidates map[int32]interfaceCandidate) (interfaceCandidate, bool) {
	var found interfaceCandidate
	var physicalCount int
	var sharedMAC string
	for _, candidate := range candidates {
		if candidate.macAddress == "" {
			return interfaceCandidate{}, false
		}
		if sharedMAC == "" {
			sharedMAC = candidate.macAddress
		} else if candidate.macAddress != sharedMAC {
			return interfaceCandidate{}, false
		}
		if candidate.isPhysical {
			found = candidate
			physicalCount++
			if physicalCount > 1 {
				return interfaceCandidate{}, false
			}
		}
	}
	return found, physicalCount == 1
}

func buildInterfaceIndexByIDType(interfaces []devicemetadata.InterfaceMetadata) map[string]map[string][]interfaceCandidate {
	interfaceIndexByIDType := make(map[string]map[string][]interfaceCandidate)
	for _, idType := range []string{"mac_address", "interface_name", "interface_alias", "interface_index"} {
		interfaceIndexByIDType[idType] = make(map[string][]interfaceCandidate)
	}

	for _, devInterface := range interfaces {
		isPhysical := devInterface.IsPhysical != nil && *devInterface.IsPhysical
		candidate := interfaceCandidate{
			ifIndex:    devInterface.Index,
			isPhysical: isPhysical,
			macAddress: devInterface.MacAddress,
		}

		interfaceIndexByIDType["mac_address"][devInterface.MacAddress] = append(interfaceIndexByIDType["mac_address"][devInterface.MacAddress], candidate)
		interfaceIndexByIDType["interface_name"][devInterface.Name] = append(interfaceIndexByIDType["interface_name"][devInterface.Name], candidate)
		interfaceIndexByIDType["interface_alias"][devInterface.Alias] = append(interfaceIndexByIDType["interface_alias"][devInterface.Alias], candidate)
		interfaceIndexByIDType["interface_index"][formatInt32(devInterface.Index)] = append(interfaceIndexByIDType["interface_index"][formatInt32(devInterface.Index)], candidate)
	}

	return interfaceIndexByIDType
}

func formatInt32(value int32) string {
	return strconv.FormatInt(int64(value), 10)
}
