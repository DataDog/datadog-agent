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

		device_id := buildDeviceID(namespace, device.IPAddress)

		// Will have to iterate over all connections, building links for each
		RemoteSystemName:=""
		RemoteSystemDescription:=""
		id:=""
		idType:=""
		RemoteIpAddress:=""
		RemotePortId:=""
		RemotePortDescription:=""

		local := generate_local_side(device_id, "")
		remote := generate_remote_side(
            RemoteSystemName,
            RemoteSystemDescription,
            id,
            idType,
            RemoteIpAddress,
            RemotePortId,
            RemotePortDescription,
        )

        link := devicemetadata.TopologyLinkMetadata{
            ID: generate_topology_link_id(device_id, "", RemotePortId),
            SourceType: "lldp",
            Integration: "versa",
            Local: &local,
            Remote: &remote,
        }

		links = append(links, link)
	}

	return links, nil
}

func generate_local_side(device_id string, local_port_id string) devicemetadata.TopologyLinkSide {
    return devicemetadata.TopologyLinkSide{
        Device: &devicemetadata.TopologyLinkDevice{
            DDID: device_id,
        },
        Interface: &devicemetadata.TopologyLinkInterface{
            DDID: generate_interface_dd_id(),
            ID: local_port_id,
            IDType: "",
        },
    }
}

func generate_remote_side(sys_name string, sys_desc string, id string, id_type string, ip_addr string, port_id string, port_desc string) devicemetadata.TopologyLinkSide {
    return devicemetadata.TopologyLinkSide{
        Device: &devicemetadata.TopologyLinkDevice{
            DDID: generate_device_dd_id(),
            Name: sys_name,
            Description: sys_desc,
            ID: id,
            IDType: id_type,
            IPAddress: ip_addr,
        },
        Interface: generate_remote_interface(remote_device, port_id, port_desc),
	}
}


func generate_remote_interface(port_id string, port_desc string) *devicemetadata.TopologyLinkInterface {
	remote_interface := devicemetadata.TopologyLinkInterface{
		DDID: generate_interface_dd_id(),
		ID: port_id,
		IDType: "interface_name",
		Description: port_desc,
	}
	return &remote_interface
}

func generate_topology_link_id(local_device_id string, local_port_id string, remote_port_id string) string {
    //Generate the topology link ID according to NDM format
    return fmt.Sprintf("%s:%s.%s", local_device_id, local_port_id, remote_port_id)
}

func generate_interface_dd_id() string {
    //Generate the device's interface ID if possible (Datadog monitored device and interface)
    return ""
}
