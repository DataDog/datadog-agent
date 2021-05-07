package snmp

import (
	"github.com/DataDog/agent-payload/network-devices"
	"github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (ms *metricSender) reportNetworkDeviceMetadata(config snmpConfig, store *resultValueStore, tags []string) {
	var vendor string

	log.Debugf("[DEV] Reporting NetworkDevicesMetadata")

	sysName := getScalarString(store, sysNameOID)
	sysDescr := getScalarString(store, sysDescrOID)
	sysObjectID := getScalarString(store, sysObjectIDOID)

	if config.profileDef != nil {
		vendor = config.profileDef.Device.Vendor
	}

	deviceMessage := &network_devices.CollectorNetworkDevice{
		Device: &network_devices.NetworkDevice{
			Id:          "abc123",
			Name:        sysName,
			Description: sysDescr,
			IpAddress:   config.ipAddress,
			SysObjectId: sysObjectID,
			Profile:     config.profile,
			Vendor:      vendor,
			Tags:        tags,
		},
	}

	log.Debugf("[DEV] deviceMessage: %v", deviceMessage)

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

func getScalarString(store *resultValueStore, oid string) string {
	value, err := store.getScalarValue(oid)
	if err != nil {
		log.Debugf("failed to get value for OID %s: %s", oid, err)
		return ""
	}
	str, err := value.toString()
	if err != nil {
		log.Debugf("failed to convert to string for OID %s with value %v: %s", oid, value, err)
		return ""
	}
	return str
}
