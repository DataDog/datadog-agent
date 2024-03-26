// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package payload

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

// CiscoInterface is an interface to abstract the underlying network interface object (vEdge vs cEdge)
type CiscoInterface interface {
	// ID returns an unique interface ID
	ID() string
	// Index returns the interface index
	Index() int
	// Speed returns the interface speed
	Speed() int
	// OperStatus returns the interface oper status
	OperStatus() devicemetadata.IfOperStatus
	// AdminStatus returns the interface admin status
	AdminStatus() devicemetadata.IfAdminStatus
	// Metadata returns the interface metadata
	Metadata(namespace string) devicemetadata.InterfaceMetadata
	// IPAddressMetadata returns the metadata for this interface's IP addresses
	IPAddressMetadata(namespace string) (devicemetadata.IPAddressMetadata, error)
}

// ProcessInterfaces process interfaces API payloads to build metadata and tags
func ProcessInterfaces(namespace string, vEdgeInterfaces []client.InterfaceState, cEdgeInterfaces []client.CEdgeInterfaceState) (interfaces []devicemetadata.InterfaceMetadata, ipAddresses []devicemetadata.IPAddressMetadata, interfacesMap map[string]CiscoInterface) {
	interfacesMap = make(map[string]CiscoInterface)

	itfs := convertInterfaces(vEdgeInterfaces, cEdgeInterfaces)

	for _, itf := range itfs {
		_, present := interfacesMap[itf.ID()]

		// Avoid sending duplicated interface metadata (In case the interface is returned both for IPv4 and IPv6)
		if !present {
			interfaces = append(interfaces, itf.Metadata(namespace))
			interfacesMap[itf.ID()] = itf
		}
		if ipAddress, err := itf.IPAddressMetadata(namespace); err == nil {
			ipAddresses = append(ipAddresses, ipAddress)
		}
	}

	return interfaces, ipAddresses, interfacesMap
}

func convertInterfaces(vEdgeInterfaces []client.InterfaceState, cEdgeInterfaces []client.CEdgeInterfaceState) []CiscoInterface {
	var interfaces []CiscoInterface
	for _, itf := range vEdgeInterfaces {
		interfaces = append(interfaces, &VEdgeInterface{itf})
	}
	for _, itf := range cEdgeInterfaces {
		interfaces = append(interfaces, &CEdgeInterface{itf})
	}
	return interfaces
}

func convertOperStatus(statusMap map[string]devicemetadata.IfOperStatus, status string) devicemetadata.IfOperStatus {
	operStatus, ok := statusMap[status]
	if !ok {
		operStatus = devicemetadata.OperStatusUnknown
	}
	return operStatus
}

func convertAdminStatus(statusMap map[string]devicemetadata.IfAdminStatus, status string) devicemetadata.IfAdminStatus {
	adminStatus, ok := statusMap[status]
	if !ok {
		adminStatus = devicemetadata.AdminStatusDown
	}
	return adminStatus
}
