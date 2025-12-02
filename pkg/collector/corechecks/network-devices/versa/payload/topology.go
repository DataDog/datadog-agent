// Package payload implement processing of Versa api responses
package payload

import (

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"fmt"

)

// GetDeviceMetadataFromAppliances process devices API payloads to build metadata
func GetTopologyMetadata(namespace string, deviceNameToIPMap map[string]string, appliances []client.Appliance) ([]devicemetadata.TopologyLinkMetadata, error) {

	var links []devicemetadata.TopologyLinkMetadata

	// Iterate over all appliances, build topology for each device
	for _, device := range appliances {

		// Will have to iterate over/unpack all of the connections to the current device

		// Collect the data needed to build the links
		localDeviceId := buildDeviceID(namespace, device.IPAddress)
		localPortId:=""
		localPortIdType:=""
		
		remoteSystemName:=""
		remoteSystemDescription:=""
		remoteDeviceId:=""
		remoteDeviceIdType:=""
		remoteIpAddress:=""
		remotePortId:=""
		remotePortIdType:=""
		remotePortDescription:=""

		var remoteLink *devicemetadata.TopologyLinkSide

		if datadogDevice(){ //if the remote device is monitored by Datadog, create remote structs with DDIDs
			remoteDeviceDDID := generate_device_dd_id()
			remoteInterfaceDDID := generate_interface_dd_id()

			remoteLink = &devicemetadata.TopologyLinkSide{
				Device: &devicemetadata.TopologyLinkDevice{
					DDID: remoteDeviceDDID,
					Name: remoteSystemName,
					Description: remoteSystemDescription,
					ID: remoteDeviceId,
					IDType: remoteDeviceIdType,
					IPAddress: remoteIpAddress,
				},
				Interface: &devicemetadata.TopologyLinkInterface{
					DDID: remoteInterfaceDDID,
					ID: remotePortId,
					IDType: remotePortIdType,
					Description: remotePortDescription,
				},
			}
		} else { //if the remote device is not monitored by Datadog, create remote structs without DDIDs
			remoteLink = &devicemetadata.TopologyLinkSide{
				Device: &devicemetadata.TopologyLinkDevice{
					Name: remoteSystemName,
					Description: remoteSystemDescription,
					ID: remoteDeviceId,
					IDType: remoteDeviceIdType,
					IPAddress: remoteIpAddress,
				},
				Interface: &devicemetadata.TopologyLinkInterface{
					ID: remotePortId,
					IDType: remotePortIdType,
					Description: remotePortDescription,
				},
			}
		}

		// create the link
        link := devicemetadata.TopologyLinkMetadata{
            ID: generate_topology_link_id(localDeviceId, localPortId, remotePortId),
            SourceType: "lldp",
            Integration: "versa",
            Local: &devicemetadata.TopologyLinkSide{
				Device: &devicemetadata.TopologyLinkDevice{
					DDID: localDeviceId,
				},
				Interface: &devicemetadata.TopologyLinkInterface{
					DDID: generate_interface_dd_id(),
					ID: localPortId,
					IDType: localPortIdType,
				},
			},
            Remote: remoteLink,
        }

		links = append(links, link)
	}

	return links, nil
}

func generate_topology_link_id(local_device_id string, local_port_id string, remote_port_id string) string {
    //Generate the topology link ID according to NDM format
    return fmt.Sprintf("%s:%s.%s", local_device_id, local_port_id, remote_port_id)
}

func datadogDevice() bool {
    //Check if the device is monitored by Datadog
    return true
}

func generate_device_dd_id() string {
    //Generate the device's interface ID if possible (Datadog monitored device and interface)
    return ""
}

func generate_interface_dd_id() string {
    //Generate the device's interface ID if possible (Datadog monitored device and interface)
    return ""
}
