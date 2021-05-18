package snmp

import (
	json "encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"sort"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/metadata"
)

func (ms *metricSender) reportNetworkDeviceMetadata(config snmpConfig, store *resultValueStore, tags []string) {
	log.Debugf("[DEV] Reporting NetworkDevicesMetadata")

	deviceID, deviceIDTags := buildDeviceID(config.getDeviceIDTags())

	device := ms.buildNetworkDeviceMetadata(deviceID, deviceIDTags, config, store, tags)

	interfaces, err := ms.buildNetworkInterfacesMetadata(deviceID, config, store, tags)
	if err != nil {
		log.Errorf("Error building interfaces metadata: %s", err)
	}

	metadata := metadata.NetworkDevicesMetadata{
		Devices: []metadata.DeviceMetadata{
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

func (ms *metricSender) buildNetworkDeviceMetadata(deviceID string, idTags []string, config snmpConfig, store *resultValueStore, tags []string) metadata.DeviceMetadata {
	var vendor string
	sysName := store.getScalarValueAsString(metadata.SysNameOID)
	sysDescr := store.getScalarValueAsString(metadata.SysDescrOID)
	sysObjectID := store.getScalarValueAsString(metadata.SysObjectIDOID)

	if config.profileDef != nil {
		vendor = config.profileDef.Device.Vendor
	}
	sort.Strings(tags)
	return metadata.DeviceMetadata{
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

func (ms *metricSender) buildNetworkInterfacesMetadata(deviceID string, config snmpConfig, store *resultValueStore, tags []string) ([]metadata.InterfaceMetadata, error) {
	indexes, err := store.getColumnIndexes(metadata.IfNameOID)
	if err != nil {
		return nil, fmt.Errorf("error getting indexes: %s", err)
	}

	var interfaces []metadata.InterfaceMetadata
	for _, strIndex := range indexes {
		index, err := strconv.Atoi(strIndex)
		if err != nil {
			log.Warnf("interface metadata: invalid index: %s", index)
			continue
		}

		networkInterface := metadata.InterfaceMetadata{
			DeviceID:    deviceID,
			Index:       int32(index),
			Name:        store.getColumnValueAsString(metadata.IfNameOID, strIndex),
			Alias:       store.getColumnValueAsString(metadata.IfAliasOID, strIndex),
			Description: store.getColumnValueAsString(metadata.IfDescrOID, strIndex),
			MacAddress:  store.getColumnValueAsString(metadata.IfPhysAddressOID, strIndex),
			AdminStatus: int32(store.getColumnValueAsFloat(metadata.IfAdminStatusOID, strIndex)),
			OperStatus:  int32(store.getColumnValueAsFloat(metadata.IfOperStatusOID, strIndex)),
		}
		interfaces = append(interfaces, networkInterface)
	}
	return interfaces, err
}
