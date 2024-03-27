// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package payload

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// CiscoInterface is an interface to abstract the underlying network interface object (vEdge vs cEdge)
type CiscoInterface interface {
	// ID returns an unique interface ID
	ID() string
	// Index returns the interface index
	Index() (int, error)
	// Speed returns the interface speed
	Speed() int
	// OperStatus returns the interface oper status
	OperStatus() devicemetadata.IfOperStatus
	// AdminStatus returns the interface admin status
	AdminStatus() devicemetadata.IfAdminStatus
	// Metadata returns the interface metadata
	Metadata(namespace string) (devicemetadata.InterfaceMetadata, error)
	// IPAddressMetadata returns the metadata for this interface's IP addresses
	IPAddressMetadata(namespace string) (devicemetadata.IPAddressMetadata, error)
}

// GetInterfacesMetadata process interfaces API payloads to build interfaces metadata and tags
func GetInterfacesMetadata(namespace string, interfaces []CiscoInterface) ([]devicemetadata.InterfaceMetadata, map[string]CiscoInterface) {
	var interfacesMetadata []devicemetadata.InterfaceMetadata
	interfacesMap := make(map[string]CiscoInterface)

	for _, itf := range interfaces {
		_, present := interfacesMap[itf.ID()]

		// Avoid sending duplicated interface metadata (In case the interface is returned both for IPv4 and IPv6)
		if !present {
			interfaceMetadata, err := itf.Metadata(namespace)
			if err != nil {
				log.Warnf("Unable process interface metadata for %s : %s", itf.ID(), err)
				continue
			}

			interfacesMetadata = append(interfacesMetadata, interfaceMetadata)
			interfacesMap[itf.ID()] = itf
		}
	}

	return interfacesMetadata, interfacesMap
}

// GetIPAddressesMetadata process interfaces API payloads to build IP addresses metadata
func GetIPAddressesMetadata(namespace string, interfaces []CiscoInterface) []devicemetadata.IPAddressMetadata {
	var ipAddresses []devicemetadata.IPAddressMetadata
	for _, itf := range interfaces {
		ipAddress, err := itf.IPAddressMetadata(namespace)
		if err != nil {
			log.Warnf("Unable to process IP address metadata for %s : %s", itf.ID(), err)
			continue
		}
		ipAddresses = append(ipAddresses, ipAddress)
	}
	return ipAddresses
}

// ConvertInterfaces converts API responses to an abstracted interface
func ConvertInterfaces(vEdgeInterfaces []client.InterfaceState, cEdgeInterfaces []client.CEdgeInterfaceState) []CiscoInterface {
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
