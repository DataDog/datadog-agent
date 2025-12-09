// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package payload implement processing of Versa api responses
package payload

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

// GetTopologyMetadata process a list of device neighbors into topology links
func GetTopologyMetadata(namespace string, deviceNameToIPMap map[string]string, device client.Appliance, neighbors []client.Neighbor) ([]devicemetadata.TopologyLinkMetadata, error) {

	var links []devicemetadata.TopologyLinkMetadata

	// Iterate over all appliances, build topology for each device
	for _, neighbor := range neighbors {

		// Will have to iterate over/unpack all of the connections to the current device

		// Collect the data needed to build the links
		localDeviceID := buildDeviceID(namespace, device.IPAddress)
		localPortID := ""
		localPortIDType := ""

		var remoteLink *devicemetadata.TopologyLinkSide

		if datadogDevice(neighbor.SystemName, neighbor.IPAddress, deviceNameToIPMap) { //if the remote device is monitored by Datadog, create remote structs with DDIDs
			remoteDeviceDDID := buildDeviceID(namespace, neighbor.IPAddress)
			remoteInterfaceDDID := generateInterfaceDDID()

			remoteLink = &devicemetadata.TopologyLinkSide{
				Device: &devicemetadata.TopologyLinkDevice{
					DDID:        remoteDeviceDDID,
					Name:        neighbor.SystemName,
					Description: neighbor.SystemDescription,
					ID:          neighbor.ChassisID,
					IDType:      neighbor.DeviceIDType,
					IPAddress:   neighbor.IPAddress,
				},
				Interface: &devicemetadata.TopologyLinkInterface{
					DDID:        remoteInterfaceDDID,
					ID:          neighbor.PortID,
					IDType:      neighbor.PortIDType,
					Description: neighbor.PortDescription,
				},
			}
		} else { //if the remote device is not monitored by Datadog, create remote structs without DDIDs
			remoteLink = &devicemetadata.TopologyLinkSide{
				Device: &devicemetadata.TopologyLinkDevice{
					Name:        neighbor.SystemName,
					Description: neighbor.SystemDescription,
					ID:          neighbor.ChassisID,
					IDType:      neighbor.DeviceIDType,
					IPAddress:   neighbor.IPAddress,
				},
				Interface: &devicemetadata.TopologyLinkInterface{
					ID:          neighbor.PortID,
					IDType:      neighbor.PortIDType,
					Description: neighbor.PortDescription,
				},
			}
		}

		// create the link
		link := devicemetadata.TopologyLinkMetadata{
			ID:          generateTopologyLinkID(localDeviceID, localPortID, neighbor.PortID),
			SourceType:  "lldp",
			Integration: "versa",
			Local: &devicemetadata.TopologyLinkSide{
				Device: &devicemetadata.TopologyLinkDevice{
					DDID: localDeviceID,
				},
				Interface: &devicemetadata.TopologyLinkInterface{
					DDID:   generateInterfaceDDID(),
					ID:     localPortID,
					IDType: localPortIDType,
				},
			},
			Remote: remoteLink,
		}

		links = append(links, link)
	}

	return links, nil
}

func generateTopologyLinkID(localDeviceID string, localPortID string, remotePortID string) string {
	//Generate the topology link ID according to NDM format
	return fmt.Sprintf("%s:%s.%s", localDeviceID, localPortID, remotePortID)
}

func datadogDevice(deviceName string, deviceIP string, deviceNameToIDMap map[string]string) bool {
	//Check if the device is monitored by Datadog
	if mappedIP, ok := deviceNameToIDMap[deviceName]; ok {
		if mappedIP == deviceIP {
			return true
		}
	}
	return false
}

func generateInterfaceDDID() string {
	//Generate the device's interface ID if possible (Datadog monitored device and interface)
	return ""
}
