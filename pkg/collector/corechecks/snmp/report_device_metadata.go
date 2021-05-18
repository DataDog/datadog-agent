package snmp

import (
	json "encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"sort"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/device_metadata"
)

func (ms *metricSender) reportNetworkDeviceMetadata(config snmpConfig, store *resultValueStore, tags []string) {
	log.Debugf("[DEV] Reporting NetworkDevicesMetadata")

	deviceID, deviceIDTags := buildDeviceID(config.getDeviceIDTags())

	device := ms.buildNetworkDeviceMetadata(deviceID, deviceIDTags, config, store, tags)

	interfaces, err := ms.buildNetworkInterfacesMetadata(deviceID, config, store, tags)
	if err != nil {
		log.Errorf("Error building interfaces metadata: %s", err)
	}

	metadata := device_metadata.NetworkDevicesMetadata{
		Devices: []device_metadata.DeviceMetadata{
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

func (ms *metricSender) buildNetworkDeviceMetadata(deviceID string, idTags []string, config snmpConfig, store *resultValueStore, tags []string) device_metadata.DeviceMetadata {
	var vendor string
	sysName := store.getScalarValueAsString(device_metadata.SysNameOID)
	sysDescr := store.getScalarValueAsString(device_metadata.SysDescrOID)
	sysObjectID := store.getScalarValueAsString(device_metadata.SysObjectIDOID)

	if config.profileDef != nil {
		vendor = config.profileDef.Device.Vendor
	}
	sort.Strings(tags)
	return device_metadata.DeviceMetadata{
		ID:          deviceID,
		IDTags:      idTags,
		Name:        sysName,
		Description: sysDescr,
		IPAddress:   config.ipAddress,
		SysObjectID: sysObjectID,
		Profile:     config.profile,
		Vendor:      vendor,
		Tags:        tags,
	}
}

func (ms *metricSender) buildNetworkInterfacesMetadata(deviceID string, config snmpConfig, store *resultValueStore, tags []string) ([]device_metadata.InterfaceMetadata, error) {
	indexes, err := store.getColumnIndexes(device_metadata.IfNameOID)
	if err != nil {
		return nil, fmt.Errorf("error getting indexes: %s", err)
	}

	var interfaces []device_metadata.InterfaceMetadata
	for _, strIndex := range indexes {
		index, err := strconv.Atoi(strIndex)
		if err != nil {
			log.Warnf("interface metadata: invalid index: %s", index)
			continue
		}

		networkInterface := device_metadata.InterfaceMetadata{
			DeviceID:    deviceID,
			Index:       int32(index),
			Name:        store.getColumnValueAsString(device_metadata.IfNameOID, strIndex),
			Alias:       store.getColumnValueAsString(device_metadata.IfAliasOID, strIndex),
			Description: store.getColumnValueAsString(device_metadata.IfDescrOID, strIndex),
			MacAddress:  store.getColumnValueAsString(device_metadata.IfPhysAddressOID, strIndex),
			AdminStatus: int32(store.getColumnValueAsFloat(device_metadata.IfAdminStatusOID, strIndex)),
			OperStatus:  int32(store.getColumnValueAsFloat(device_metadata.IfOperStatusOID, strIndex)),
		}
		interfaces = append(interfaces, networkInterface)
	}
	return interfaces, err
}
