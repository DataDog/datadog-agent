// Package payload implement processing of Versa api responses
package payload

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

// GetDeviceMetadataFromAppliances process devices API payloads to build metadata
func GetTopologyMetadata(namespace string, deviceNameToIPMap map[string]string, appliances []client.Appliance) ([]devicemetadata.TopologyLinkMetadata, error) {

	var links []devicemetadata.TopologyLinkMetadata

	for _, device := range appliances {
		device_id := buildDeviceID(namespace, device.IPAddress)

		// Build topology data for the local device, and the remote device that is connected
		local = generate_local_side(device_id, device_name: device.Name, device_description: device.Description, device_ip: device.IPAddress)
		remote = generate_remote_side()

		newLink := TopologyLinkMetadata{
			id=generate_topology_link_id(local_device_id, local_port_id, remote_port_id),
            source_type="",
            integration="versa",
            local=local
            remote=remote
		}
		links = append(links, newLink)
	}

	return links, nil
}

func generate_local_side(local_device_id string, ) {
	return TopologyLinkSide{
        device=TopologyLinkDevice(dd_id=device_id),
        interface=TopologyLinkInterface{
            dd_id=generate_interface_dd_id(),
            id=,
            id_type="",
		}
		}
}

func generate_remote_side() {
	// Check if the device is monitored by Datadog, if it is, can use a DD ID
	remote_device_dd_id = None
    if remote_device:
        remote_device_dd_id = remote_device.serial

    return TopologyLinkSide(
        device=TopologyLinkDevice(
            dd_id=remote_device_dd_id,
            name=sys_name,
            description=sys_desc,
            id=id,
            id_type=id_type,
            ip_address=ip_addr,
        ),
        interface=generate_remote_interface(remote_device, port_id, port_desc)
}

func generate_remote_interface(remote_device client.Device, port_id string, port_desc string) {
	
	remote_interface = TopologyLinkInterface{id=f"Port {port_id}", id_type="interface_name", description=port_desc}
    
	if remote_device:
        remote_interface_dd_id, ok = generate_interface_dd_id()

        if ok:
            remote_interface = TopologyLinkInterface{dd_id=remote_interface_dd_id}
        else:
            remote_interface = TopologyLinkInterface{id=, id_type=""}

    return remote_interface
}

// generate_interface_dd_id returns the Datadog ID for the interface, error indicates the interface does not resolve to device that is monitored by Datadog
func generate_interface_dd_id() (id string, error) {

	return nil, errors.New("Not a Datadog monitored device")
}

func generate_topology_link_id(local_device_id string, local_port_id string, remote_port_id string) string {
	return f"{local_device_id}:{local_port_id}.{remote_port_id}"
}
