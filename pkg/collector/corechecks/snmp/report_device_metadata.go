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

	device := buildNetworkDeviceMetadata(config.deviceID, config.deviceIDTags, config, store, tags)

	interfaces, err := buildNetworkInterfacesMetadata(config.deviceID, store)
	if err != nil {
		log.Debugf("Unable to build interfaces metadata: %s", err)
		interfaces = nil
	}

	metadataPayloads := batchPayloads(config.subnet, collectTime, metadata.PayloadMetadataBatchSize, device, interfaces)

	for _, payload := range metadataPayloads {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			log.Errorf("Error marshalling device metadata: %s", err)
			return
		}
		ms.sender.EventPlatformEvent(string(payloadBytes), epforwarder.EventTypeNetworkDevicesMetadata)
	}
}

func buildNetworkDeviceMetadata(deviceID string, idTags []string, config snmpConfig, store *resultValueStore, tags []string) metadata.DeviceMetadata {
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

func buildNetworkInterfacesMetadata(deviceID string, store *resultValueStore) ([]metadata.InterfaceMetadata, error) {
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

func batchPayloads(subnet string, collectTime time.Time, batchSize int, device metadata.DeviceMetadata, interfaces []metadata.InterfaceMetadata) []metadata.NetworkDevicesMetadata {
	var payloads []metadata.NetworkDevicesMetadata
	var resourceCount int
	payload := metadata.NetworkDevicesMetadata{
		Devices: []metadata.DeviceMetadata{
			device,
		},
		Subnet:           subnet,
		CollectTimestamp: collectTime.Unix(),
	}
	resourceCount++

	for _, interfaceMetadata := range interfaces {
		if resourceCount == batchSize {
			payloads = append(payloads, payload)
			payload = metadata.NetworkDevicesMetadata{
				Subnet:           subnet,
				CollectTimestamp: collectTime.Unix(),
			}
			resourceCount = 0
		}
		resourceCount++
		payload.Interfaces = append(payload.Interfaces, interfaceMetadata)
	}

	payloads = append(payloads, payload)
	return payloads
}
