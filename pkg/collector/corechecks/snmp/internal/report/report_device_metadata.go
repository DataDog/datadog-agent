// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	json "encoding/json"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/lldp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/metadata"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

// ReportNetworkDeviceMetadata reports device metadata
func (ms *MetricSender) ReportNetworkDeviceMetadata(config *checkconfig.CheckConfig, store *valuestore.ResultValueStore, origTags []string, collectTime time.Time, deviceStatus metadata.DeviceStatus) {
	tags := common.CopyStrings(origTags)
	tags = util.SortUniqInPlace(tags)

	metadataStore := buildMetadataStore(config.Metadata, store)

	device := buildNetworkDeviceMetadata(config.DeviceID, config.DeviceIDTags, config, metadataStore, tags, deviceStatus)

	interfaces := buildNetworkInterfacesMetadata(config.DeviceID, metadataStore)
	topologyLinks := buildNetworkTopologyMetadata(config.DeviceID, metadataStore)

	metadataPayloads := batchPayloads(config.Namespace, config.ResolvedSubnetName, collectTime, metadata.PayloadMetadataBatchSize, device, interfaces, topologyLinks)

	for _, payload := range metadataPayloads {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			log.Errorf("Error marshalling device metadata: %s", err)
			return
		}
		ms.sender.EventPlatformEvent(string(payloadBytes), epforwarder.EventTypeNetworkDevicesMetadata)
	}
}

func buildMetadataStore(metadataConfigs checkconfig.MetadataConfig, values *valuestore.ResultValueStore) *metadata.Store {
	metadataStore := metadata.NewMetadataStore()
	if values == nil {
		return metadataStore
	}

	for resourceName, metadataConfig := range metadataConfigs {
		for fieldName, field := range metadataConfig.Fields {
			fieldFullName := resourceName + "." + fieldName

			var symbols []checkconfig.SymbolConfig
			if field.Symbol.OID != "" {
				symbols = append(symbols, field.Symbol)
			}
			symbols = append(symbols, field.Symbols...)

			if checkconfig.IsMetadataResourceWithScalarOids(resourceName) {
				for _, symbol := range symbols {
					if metadataStore.ScalarFieldHasValue(fieldFullName) {
						break
					}
					value, err := getScalarValueFromSymbol(values, symbol)
					if err != nil {
						log.Debugf("error getting scalar value: %v", err)
						continue
					}
					metadataStore.AddScalarValue(fieldFullName, value)

				}
				if field.Value != "" && !metadataStore.ScalarFieldHasValue(fieldFullName) {
					metadataStore.AddScalarValue(fieldFullName, valuestore.ResultValue{Value: field.Value})
				}
			} else {
				for _, symbol := range symbols {
					metricValues, err := getColumnValueFromSymbol(values, symbol)
					if err != nil {
						continue
					}
					for fullIndex, value := range metricValues {
						metadataStore.AddColumnValue(fieldFullName, fullIndex, value)
					}
				}
			}
		}
		indexOid := metadata.GetIndexOIDForResource(resourceName)
		if indexOid != "" {
			indexes, err := values.GetColumnIndexes(indexOid)
			if err != nil {
				continue
			}
			for _, fullIndex := range indexes {
				// TODO: Support extract value see II-635
				idTags := getTagsFromMetricTagConfigList(metadataConfig.IDTags, fullIndex, values)
				metadataStore.AddIDTags(resourceName, fullIndex, idTags)
			}
		}
	}
	return metadataStore
}

func buildNetworkDeviceMetadata(deviceID string, idTags []string, config *checkconfig.CheckConfig, store *metadata.Store, tags []string, deviceStatus metadata.DeviceStatus) metadata.DeviceMetadata {
	var vendor, sysName, sysDescr, sysObjectID, location, serialNumber, version, productName, model, osName, osVersion, osHostname string
	if store != nil {
		sysName = store.GetScalarAsString("device.name")
		sysDescr = store.GetScalarAsString("device.description")
		sysObjectID = store.GetScalarAsString("device.sys_object_id")
		vendor = store.GetScalarAsString("device.vendor")
		location = store.GetScalarAsString("device.location")
		serialNumber = store.GetScalarAsString("device.serial_number")
		version = store.GetScalarAsString("device.version")
		productName = store.GetScalarAsString("device.product_name")
		model = store.GetScalarAsString("device.model")
		osName = store.GetScalarAsString("device.os_name")
		osVersion = store.GetScalarAsString("device.os_version")
		osHostname = store.GetScalarAsString("device.os_hostname")
	}

	// fallback to Device.Vendor for backward compatibility
	if config.ProfileDef != nil && vendor == "" {
		vendor = config.ProfileDef.Device.Vendor
	}

	return metadata.DeviceMetadata{
		ID:           deviceID,
		IDTags:       idTags,
		Name:         sysName,
		Description:  sysDescr,
		IPAddress:    config.IPAddress,
		SysObjectID:  sysObjectID,
		Location:     location,
		Profile:      config.Profile,
		Vendor:       vendor,
		Tags:         tags,
		Subnet:       config.ResolvedSubnetName,
		Status:       deviceStatus,
		SerialNumber: serialNumber,
		Version:      version,
		ProductName:  productName,
		Model:        model,
		OsName:       osName,
		OsVersion:    osVersion,
		OsHostname:   osHostname,
	}
}

func buildNetworkInterfacesMetadata(deviceID string, store *metadata.Store) []metadata.InterfaceMetadata {
	if store == nil {
		// it's expected that the value store is nil if we can't reach the device
		// in that case, we just return a nil slice.
		return nil
	}
	indexes := store.GetColumnIndexes("interface.name")
	if len(indexes) == 0 {
		log.Debugf("Unable to build interfaces metadata: no interface indexes found")
		return nil
	}
	sort.Strings(indexes)
	var interfaces []metadata.InterfaceMetadata
	for _, strIndex := range indexes {
		index, err := strconv.ParseInt(strIndex, 10, 32)
		if err != nil {
			log.Warnf("interface metadata: invalid index: %d", index)
			continue
		}

		ifIDTags := store.GetIDTags("interface", strIndex)

		name := store.GetColumnAsString("interface.name", strIndex)
		networkInterface := metadata.InterfaceMetadata{
			DeviceID:    deviceID,
			Index:       int32(index),
			Name:        name,
			Alias:       store.GetColumnAsString("interface.alias", strIndex),
			Description: store.GetColumnAsString("interface.description", strIndex),
			MacAddress:  store.GetColumnAsString("interface.mac_address", strIndex),
			AdminStatus: int32(store.GetColumnAsFloat("interface.admin_status", strIndex)),
			OperStatus:  int32(store.GetColumnAsFloat("interface.oper_status", strIndex)),
			IDTags:      ifIDTags,
		}
		interfaces = append(interfaces, networkInterface)
	}
	return interfaces
}

func buildNetworkTopologyMetadata(deviceID string, store *metadata.Store) []metadata.TopologyLinkMetadata {
	if store == nil {
		// it's expected that the value store is nil if we can't reach the device
		// in that case, we just return a nil slice.
		return nil
	}
	indexes := store.GetColumnIndexes("lldp_remote.interface_id") // using `lldp_remote.interface_id` to get indexes since it's expected to be always present
	if len(indexes) == 0 {
		log.Debugf("Unable to build links metadata: no lldp_remote indexes found")
		return nil
	}
	sort.Strings(indexes)
	var links []metadata.TopologyLinkMetadata
	for _, strIndex := range indexes {
		indexElems := strings.Split(strIndex, ".")

		// The lldpRemEntry index is composed of those 3 elements separate by `.`: lldpRemTimeMark, lldpRemLocalPortNum, lldpRemIndex
		if len(indexElems) != 3 {
			log.Debugf("Expected 3 index elements, but got %d, index=`%s`", len(indexElems), strIndex)
			continue
		}
		// TODO: Handle TimeMark at indexElems[0] if needed later
		//       See https://www.rfc-editor.org/rfc/rfc2021

		localPortNum := indexElems[1]

		remoteDeviceIDType := lldp.ChassisIDSubtypeMap[store.GetColumnAsString("lldp_remote.chassis_id_type", strIndex)]
		remoteDeviceID := formatID(remoteDeviceIDType, store, "lldp_remote.chassis_id", strIndex)

		remoteInterfaceIDType := lldp.PortIDSubTypeMap[store.GetColumnAsString("lldp_remote.interface_id_type", strIndex)]
		remoteInterfaceID := formatID(remoteInterfaceIDType, store, "lldp_remote.interface_id", strIndex)

		localInterfaceIDType := lldp.PortIDSubTypeMap[store.GetColumnAsString("lldp_local.interface_id_type", localPortNum)]
		localInterfaceID := formatID(localInterfaceIDType, store, "lldp_local.interface_id", localPortNum)

		newLink := metadata.TopologyLinkMetadata{
			Remote: &metadata.TopologyLinkSide{
				Device: &metadata.TopologyLinkDevice{
					Name:        store.GetColumnAsString("lldp_remote.device_name", strIndex),
					Description: store.GetColumnAsString("lldp_remote.device_desc", strIndex),
					ID:          remoteDeviceID,
					IDType:      remoteDeviceIDType,
				},
				Interface: &metadata.TopologyLinkInterface{
					ID:          remoteInterfaceID,
					IDType:      remoteInterfaceIDType,
					Description: store.GetColumnAsString("lldp_remote.interface_desc", strIndex),
				},
			},
			Local: &metadata.TopologyLinkSide{
				Interface: &metadata.TopologyLinkInterface{
					ID:     localInterfaceID,
					IDType: localInterfaceIDType,
					// TODO: We can possibly resolve locally to ifIndex to avoid having to resolve on backend side
				},
				Device: &metadata.TopologyLinkDevice{
					ID:     deviceID,
					IDType: metadata.IDTypeNDM,
				},
			},
		}
		links = append(links, newLink)
	}
	return links
}

func formatID(idType string, store *metadata.Store, field string, strIndex string) string {
	var remoteDeviceID string
	if idType == metadata.IDTypeMacAddress {
		remoteDeviceID = formatColonSepBytes(store.GetColumnAsByteArray(field, strIndex))
	} else {
		remoteDeviceID = store.GetColumnAsString(field, strIndex)
	}
	return remoteDeviceID
}

func batchPayloads(namespace string, subnet string, collectTime time.Time, batchSize int, device metadata.DeviceMetadata, interfaces []metadata.InterfaceMetadata, topologyLinks []metadata.TopologyLinkMetadata) []metadata.NetworkDevicesMetadata {
	var payloads []metadata.NetworkDevicesMetadata
	var resourceCount int
	payload := metadata.NetworkDevicesMetadata{
		Devices: []metadata.DeviceMetadata{
			device,
		},
		Subnet:           subnet,
		Namespace:        namespace,
		CollectTimestamp: collectTime.Unix(),
	}
	resourceCount++

	for _, interfaceMetadata := range interfaces {
		if resourceCount == batchSize {
			payloads = append(payloads, payload)
			payload = metadata.NetworkDevicesMetadata{
				Subnet:           subnet,
				Namespace:        namespace,
				CollectTimestamp: collectTime.Unix(),
			}
			resourceCount = 0
		}
		resourceCount++
		payload.Interfaces = append(payload.Interfaces, interfaceMetadata)
	}

	for _, linkMetadata := range topologyLinks {
		if resourceCount == batchSize {
			payloads = append(payloads, payload)
			payload = metadata.NetworkDevicesMetadata{ // TODO: Avoid duplication
				Subnet:           subnet,
				Namespace:        namespace,
				CollectTimestamp: collectTime.Unix(),
			}
			resourceCount = 0
		}
		resourceCount++
		payload.Links = append(payload.Links, linkMetadata)
	}

	payloads = append(payloads, payload)
	return payloads
}
