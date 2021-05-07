package snmp

import (
	"github.com/DataDog/agent-payload/network-devices"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"strconv"
)

func (ms *metricSender) reportNetworkDeviceMetadata(config snmpConfig, store *resultValueStore, tags []string) {
	log.Debugf("[DEV] Reporting NetworkDevicesMetadata")

	deviceId := "abc123"
	ms.sendNetworkDeviceMetadata(deviceId, config, store, tags)
	ms.sendNetworkInterfaceMetadata(deviceId, config, store, tags)

}

func (ms *metricSender) sendNetworkDeviceMetadata(deviceId string, config snmpConfig, store *resultValueStore, tags []string) {
	var vendor string
	sysName := getScalarValueAsString(store, sysNameOID)
	sysDescr := getScalarValueAsString(store, sysDescrOID)
	sysObjectID := getScalarValueAsString(store, sysObjectIDOID)

	if config.profileDef != nil {
		vendor = config.profileDef.Device.Vendor
	}

	deviceMessage := &network_devices.CollectorNetworkDevice{
		Device: &network_devices.NetworkDevice{
			Id:          deviceId,
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

	ms.sender.NetworkDevicesMetadata([]serializer.ProcessMessageBody{deviceMessage}, forwarder.PayloadTypeNetworkDevice)
	// TODO: Implement CheckStats ?
}

func (ms *metricSender) sendNetworkInterfaceMetadata(deviceId string, config snmpConfig, store *resultValueStore, tags []string) {
	var interfaces []*network_devices.NetworkInterface

	// valuesByIndex is a map[<INDEX>][<OID>]snmpValueType
	valuesByIndex := make(map[string]map[string]snmpValueType)

	for _, oid := range metadataColumnOIDs {
		metricValues, err := store.getColumnValues(oid)
		if err != nil {
			log.Debugf("interface metadata: error getting column value: %v", err)
			continue
		}
		for fullIndex, value := range metricValues {
			_, ok := valuesByIndex[fullIndex]
			if !ok {
				valuesByIndex[fullIndex] = make(map[string]snmpValueType)
			}
			valuesByIndex[fullIndex][oid] = value
		}
	}

	for fullIndex, interfaceOidValues := range valuesByIndex {
		index, err := strconv.Atoi(fullIndex)
		if err != nil {
			log.Warnf("interface metadata: invalid index: %s", fullIndex)
			continue
		}

		networkInterface := network_devices.NetworkInterface{
			Index:       int32(index),
			Name:        getColumnValueAsString(interfaceOidValues, ifNameOID),
			Alias:       getColumnValueAsString(interfaceOidValues, ifAliasOID),
			Description: getColumnValueAsString(interfaceOidValues, ifDescrOID),
			MacAddress:  getColumnValueAsString(interfaceOidValues, ifPhysAddressOID),
			AdminStatus: int32(getColumnValueAsFloat(interfaceOidValues, ifAdminStatusOID)),
			OperStatus:  int32(getColumnValueAsFloat(interfaceOidValues, ifOperStatusOID)),
		}
		interfaces = append(interfaces, &networkInterface)
	}

	// TODO: batch interfaces with max
	interfacesMessage := &network_devices.CollectorNetworkInterface{
		DeviceId:   deviceId,
		Interfaces: interfaces,
	}
	log.Debugf("[DEV] interfacesMessage: %v", interfacesMessage)

	ms.sender.NetworkDevicesMetadata([]serializer.ProcessMessageBody{interfacesMessage}, forwarder.PayloadTypeNetworkInterface)
	// TODO: Implement CheckStats ?
}

func getColumnValueAsString(values map[string]snmpValueType, oid string) string {
	value, ok := values[oid]
	if !ok {
		return ""
	}
	str, err := value.toString()
	if err != nil {
		log.Debugf("failed to convert to string for OID %s with value %v: %s", oid, value, err)
		return ""
	}
	return str
}

func getColumnValueAsFloat(values map[string]snmpValueType, oid string) float64 {
	value, ok := values[oid]
	if !ok {
		return 0
	}
	floatValue, err := value.toFloat64()
	if err != nil {
		log.Debugf("failed to convert to string for OID %s with value %v: %s", oid, value, err)
		return 0
	}
	return floatValue
}

func getScalarValueAsString(store *resultValueStore, oid string) string {
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
