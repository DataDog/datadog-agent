package snmp

import (
	json "encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/metadata"
)

func (ms *metricSender) reportNetworkDeviceMetadata(config snmpConfig, store *resultValueStore, origTags []string, collectTime time.Time) {
	tags := copyStrings(origTags)
	tags = util.SortUniqInPlace(tags)

	device := ms.buildNetworkDeviceMetadata(config.deviceID, config.deviceIDTags, config, store, tags)

	interfaces, err := ms.buildNetworkInterfacesMetadata(config.deviceID, store)
	if err != nil {
		log.Debugf("Unable to build interfaces metadata: %s", err)
		interfaces = nil
	}

	metadata := metadata.NetworkDevicesMetadata{
		Devices: []metadata.DeviceMetadata{
			device,
		},
		Interfaces:       interfaces,
		Subnet:           config.subnet,
		CollectTimestamp: collectTime.Unix(),
	}
	metadataBytes, err := json.Marshal(metadata)
	if err != nil {
		log.Errorf("Error marshalling device metadata: %s", err)
		return
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
		Subnet:      config.subnet,
	}
}

func (ms *metricSender) buildNetworkInterfacesMetadata(deviceID string, store *resultValueStore) ([]metadata.InterfaceMetadata, error) {
	indexes, err := store.getColumnIndexes(metadata.IfNameOID)
	if err != nil {
		return nil, fmt.Errorf("no interface indexes found: %s", err)
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
