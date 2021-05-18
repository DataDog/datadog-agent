package snmp

import (
	json "encoding/json"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"sort"
	"strconv"
)

func (ms *metricSender) reportNetworkDeviceMetadata(config snmpConfig, store *resultValueStore, tags []string) {
	log.Debugf("[DEV] Reporting NetworkDevicesMetadata")

	deviceID := "abc123"

	device := ms.buildNetworkDeviceMetadata(deviceID, config, store, tags)
	interfaces := ms.buildNetworkInterfacesMetadata(deviceID, config, store, tags)
	metadata := NetworkDevicesMetadata{
		Devices: []DeviceMetadata{
			device,
		},
		Interfaces: interfaces,
	}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		log.Errorf("Error marshalling device metadata: %s", err)
	}
	ms.sender.EventPlatformEvent(string(metadataBytes), epforwarder.EventTypeNetworkDevicesMetadata)
}

func (ms *metricSender) buildNetworkDeviceMetadata(deviceID string, config snmpConfig, store *resultValueStore, tags []string) DeviceMetadata {
	var vendor string
	sysName := getScalarValueAsString(store, sysNameOID)
	sysDescr := getScalarValueAsString(store, sysDescrOID)
	sysObjectID := getScalarValueAsString(store, sysObjectIDOID)

	if config.profileDef != nil {
		vendor = config.profileDef.Device.Vendor
	}
	sort.Strings(tags)
	return DeviceMetadata{
		ID:          deviceID,
		Name:        sysName,
		Description: sysDescr,
		IPAddress:   config.ipAddress,
		SysObjectID: sysObjectID,
		Profile:     config.profile,
		Vendor:      vendor,
		Tags:        tags,
	}
}

func (ms *metricSender) buildNetworkInterfacesMetadata(deviceID string, config snmpConfig, store *resultValueStore, tags []string) []InterfaceMetadata {
	var interfaces []InterfaceMetadata

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

	var indexes []string
	for index := range valuesByIndex {
		indexes = append(indexes, index)
	}
	sort.Strings(indexes)
	for _, strIndex := range indexes {
		interfaceOidValues := valuesByIndex[strIndex]
		index, err := strconv.Atoi(strIndex)
		if err != nil {
			log.Warnf("interface metadata: invalid index: %s", index)
			continue
		}

		networkInterface := InterfaceMetadata{
			DeviceID:    deviceID,
			Index:       int32(index),
			Name:        getColumnValueAsString(interfaceOidValues, ifNameOID),
			Alias:       getColumnValueAsString(interfaceOidValues, ifAliasOID),
			Description: getColumnValueAsString(interfaceOidValues, ifDescrOID),
			MacAddress:  getColumnValueAsString(interfaceOidValues, ifPhysAddressOID),
			AdminStatus: int32(getColumnValueAsFloat(interfaceOidValues, ifAdminStatusOID)),
			OperStatus:  int32(getColumnValueAsFloat(interfaceOidValues, ifOperStatusOID)),
		}
		interfaces = append(interfaces, networkInterface)
	}
	return interfaces
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
