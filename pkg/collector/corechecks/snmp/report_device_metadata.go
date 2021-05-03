package snmp

import (
	"github.com/DataDog/agent-payload/network-devices"
	"github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (ms *metricSender) reportNetworkDeviceMetadata(store *resultValueStore, tags []string) {
	log.Debugf("[DEV] Reporting NetworkDevicesMetadata")

	deviceMessage := &network_devices.CollectorNetworkDevice{
		Device: &network_devices.NetworkDevice{
			Id:          "abc123",
			Name:        "my-Name",
			Description: "my-Description",
			IpAddress:   "1.2.3.4",
			SysObjectId: "1.2.3.4.5.6.6.7.8",
			Profile:     "my-profile",
			Vendor:      "my-vendor",
			Tags:        tags,
		},
	}
	ms.sendNetworkDeviceMetadata(deviceMessage)

	interfaces := &network_devices.CollectorNetworkInterface{
		DeviceId: "abc123",
		Interfaces: []*network_devices.NetworkInterface{
			{
				Index:       1,
				Name:        "interface-1",
				Alias:       "alias-1",
				Description: "interface-1-desc",
				MacAddress:  "3c:22:fb:40:b4:a1",
				AdminStatus: 1,
				OperStatus:  1,
			},
			{
				Index:       2,
				Name:        "interface-2",
				Alias:       "alias-2",
				Description: "interface-2-desc",
				MacAddress:  "3c:22:fb:40:b4:a2",
				AdminStatus: 2,
				OperStatus:  2,
			},
		},
	}
	ms.sendNetworkDeviceMetadata(interfaces)

}

func (ms *metricSender) sendNetworkDeviceMetadata(clusterMessage process.MessageBody) {
	ms.sender.NetworkDevicesMetadata([]serializer.ProcessMessageBody{clusterMessage}, forwarder.PayloadTypeNetworkDevice)
	// TODO: Implement CheckStats ?
}

func (ms *metricSender) sendNetworkInterfaceMetadata(clusterMessage process.MessageBody) {
	ms.sender.NetworkDevicesMetadata([]serializer.ProcessMessageBody{clusterMessage}, forwarder.PayloadTypeNetworkInterface)
	// TODO: Implement CheckStats ?
}
