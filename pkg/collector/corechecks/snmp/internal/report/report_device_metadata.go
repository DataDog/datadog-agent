// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	json "encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/lldp"

	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/metadata"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

const interfaceStatusMetric = "snmp.interface.status"
const topologyLinkSourceTypeLLDP = "lldp"

// ReportNetworkDeviceMetadata reports device metadata
func (ms *MetricSender) ReportNetworkDeviceMetadata(config *checkconfig.CheckConfig, store *valuestore.ResultValueStore, origTags []string, collectTime time.Time, deviceStatus metadata.DeviceStatus) {
	tags := common.CopyStrings(origTags)
	tags = util.SortUniqInPlace(tags)

	metadataStore := buildMetadataStore(config.Metadata, store)

	device := buildNetworkDeviceMetadata(config.DeviceID, config.DeviceIDTags, config, metadataStore, tags, deviceStatus)

	interfaces := buildNetworkInterfacesMetadata(config.DeviceID, metadataStore)
	ipAddresses := buildNetworkIPAddressesMetadata(config.DeviceID, metadataStore)
	topologyLinks := buildNetworkTopologyMetadata(config.DeviceID, metadataStore, interfaces)

	metadataPayloads := batchPayloads(config.Namespace, config.ResolvedSubnetName, collectTime, metadata.PayloadMetadataBatchSize, device, interfaces, ipAddresses, topologyLinks)

	for _, payload := range metadataPayloads {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			log.Errorf("Error marshalling device metadata: %s", err)
			return
		}
		ms.sender.EventPlatformEvent(payloadBytes, epforwarder.EventTypeNetworkDevicesMetadata)
	}

	// Telemetry
	for _, interfaceStatus := range interfaces {
		status := string(computeInterfaceStatus(interfaceStatus.AdminStatus, interfaceStatus.OperStatus))
		interfaceTags := []string{
			"status:" + status,
			"admin_status:" + interfaceStatus.AdminStatus.AsString(),
			"oper_status:" + interfaceStatus.OperStatus.AsString(),
			"interface_index:" + strconv.Itoa(int(interfaceStatus.Index)),
		}
		if interfaceStatus.Name != "" {
			interfaceTags = append(interfaceTags, "interface:"+interfaceStatus.Name)
		}
		if interfaceStatus.Alias != "" {
			interfaceTags = append(interfaceTags, "interface_alias:"+interfaceStatus.Alias)
		}
		interfaceTags = append(interfaceTags, tags...)
		ms.sender.Gauge(interfaceStatusMetric, 1, "", interfaceTags)
	}
}

func computeInterfaceStatus(adminStatus common.IfAdminStatus, operStatus common.IfOperStatus) common.InterfaceStatus {
	if adminStatus == common.AdminStatus_Up {
		switch {
		case operStatus == common.OperStatus_Up:
			return common.InterfaceStatus_Up
		case operStatus == common.OperStatus_Down:
			return common.InterfaceStatus_Down
		}
		return common.InterfaceStatus_Warning
	}
	if adminStatus == common.AdminStatus_Down {
		switch {
		case operStatus == common.OperStatus_Up:
			return common.InterfaceStatus_Down
		case operStatus == common.OperStatus_Down:
			return common.InterfaceStatus_Off
		}
		return common.InterfaceStatus_Warning
	}
	if adminStatus == common.AdminStatus_Testing {
		switch {
		case operStatus != common.OperStatus_Down:
			return common.InterfaceStatus_Warning
		}
	}
	return common.InterfaceStatus_Down
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
			AdminStatus: common.IfAdminStatus((store.GetColumnAsFloat("interface.admin_status", strIndex))),
			OperStatus:  common.IfOperStatus((store.GetColumnAsFloat("interface.oper_status", strIndex))),
			IDTags:      ifIDTags,
		}
		interfaces = append(interfaces, networkInterface)
	}
	return interfaces
}

func buildNetworkIPAddressesMetadata(deviceID string, store *metadata.Store) []metadata.IPAddressMetadata {
	if store == nil {
		// it's expected that the value store is nil if we can't reach the device
		// in that case, we just return a nil slice.
		return nil
	}
	indexes := store.GetColumnIndexes("ip_addresses.if_index")
	if len(indexes) == 0 {
		log.Debugf("Unable to build ip addresses metadata: no ip_addresses.if_index found")
		return nil
	}
	sort.Strings(indexes)
	var ipAddresses []metadata.IPAddressMetadata
	for _, strIndex := range indexes {
		index := store.GetColumnAsString("ip_addresses.if_index", strIndex)
		Netmask := store.GetColumnAsString("ip_addresses.netmask", strIndex)
		ipAddress := metadata.IPAddressMetadata{
			InterfaceID: deviceID + ":" + index,
			IPAddress:   strIndex,
			Prefixlen:   int32(netmaskToPrefixlen(Netmask)),
		}
		ipAddresses = append(ipAddresses, ipAddress)
	}
	return ipAddresses
}

func buildNetworkTopologyMetadata(deviceID string, store *metadata.Store, interfaces []metadata.InterfaceMetadata) []metadata.TopologyLinkMetadata {
	if store == nil {
		// it's expected that the value store is nil if we can't reach the device
		// in that case, we just return a nil slice.
		return nil
	}

	interfaceIndexByIDType := buildInterfaceIndexByIDType(interfaces)

	remManAddrByLLDPRemIndex := getRemManIPAddrByLLDPRemIndex(store.GetColumnIndexes("lldp_remote_management.interface_id_type"))

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
		lldpRemIndex := indexElems[2]

		remoteDeviceIDType := lldp.ChassisIDSubtypeMap[store.GetColumnAsString("lldp_remote.chassis_id_type", strIndex)]
		remoteDeviceID := formatID(remoteDeviceIDType, store, "lldp_remote.chassis_id", strIndex)

		remoteInterfaceIDType := lldp.PortIDSubTypeMap[store.GetColumnAsString("lldp_remote.interface_id_type", strIndex)]
		remoteInterfaceID := formatID(remoteInterfaceIDType, store, "lldp_remote.interface_id", strIndex)

		localInterfaceIDType := lldp.PortIDSubTypeMap[store.GetColumnAsString("lldp_local.interface_id_type", localPortNum)]
		localInterfaceID := formatID(localInterfaceIDType, store, "lldp_local.interface_id", localPortNum)

		resolvedLocalInterfaceID := resolveLocalInterface(deviceID, interfaceIndexByIDType, localInterfaceIDType, localInterfaceID)

		// remEntryUniqueID: The combination of localPortNum and lldpRemIndex is expected to be unique for each entry in
		//                   lldpRemTable. We don't include lldpRemTimeMark (used for filtering only recent data) since it can change often.
		remEntryUniqueID := localPortNum + "." + lldpRemIndex

		newLink := metadata.TopologyLinkMetadata{
			ID:         deviceID + ":" + remEntryUniqueID,
			SourceType: topologyLinkSourceTypeLLDP,
			Remote: &metadata.TopologyLinkSide{
				Device: &metadata.TopologyLinkDevice{
					Name:        store.GetColumnAsString("lldp_remote.device_name", strIndex),
					Description: store.GetColumnAsString("lldp_remote.device_desc", strIndex),
					ID:          remoteDeviceID,
					IDType:      remoteDeviceIDType,
					IPAddress:   remManAddrByLLDPRemIndex[lldpRemIndex],
				},
				Interface: &metadata.TopologyLinkInterface{
					ID:          remoteInterfaceID,
					IDType:      remoteInterfaceIDType,
					Description: store.GetColumnAsString("lldp_remote.interface_desc", strIndex),
				},
			},
			Local: &metadata.TopologyLinkSide{
				Interface: &metadata.TopologyLinkInterface{
					DDID:   resolvedLocalInterfaceID,
					ID:     localInterfaceID,
					IDType: localInterfaceIDType,
				},
				Device: &metadata.TopologyLinkDevice{
					DDID: deviceID,
				},
			},
		}
		links = append(links, newLink)
	}
	return links
}

func resolveLocalInterface(deviceID string, interfaceIndexByIDType map[string]map[string][]int32, localInterfaceIDType string, localInterfaceID string) string {
	if localInterfaceID == "" {
		return ""
	}

	var typesToTry []string
	if localInterfaceIDType == "" {
		// "smart resolution" by multiple types when localInterfaceIDType is not provided (which is often the case).
		// CAVEAT: In case the smart resolution returns false positives, the solution is to configure the device to provide a proper localInterfaceIDType.
		// The order of `typesToTry` has been arbitrary define (not sure if there is an order that can lead to lower false positive).
		typesToTry = []string{"mac_address", "interface_name", "interface_alias", "interface_index"}
	} else {
		typesToTry = []string{localInterfaceIDType}
	}
	matchedIfIndexesMap := make(map[int32]struct{})
	for _, idType := range typesToTry {
		interfaceIndexByIDValue, ok := interfaceIndexByIDType[idType]
		if ok {
			ifIndexes, ok := interfaceIndexByIDValue[localInterfaceID]
			if ok {
				for _, ifIndex := range ifIndexes {
					matchedIfIndexesMap[ifIndex] = struct{}{}
				}
			}
		}
	}
	if len(matchedIfIndexesMap) == 1 {
		var matchedIfIndexes []int32
		for key := range matchedIfIndexesMap {
			matchedIfIndexes = append(matchedIfIndexes, key)
		}
		interfaceID := deviceID + ":" + strconv.Itoa(int(matchedIfIndexes[0]))
		log.Tracef("[local interface resolution] found 1 matching interface (idType=%s, id=%s) resolved to interface_id `%s`", localInterfaceIDType, localInterfaceID, interfaceID)
		return interfaceID
	} else if len(matchedIfIndexesMap) > 1 {
		log.Tracef("[local interface resolution] expected 1 matching interface but found %d (idType=%s, id=%s): %+v", len(matchedIfIndexesMap), localInterfaceIDType, localInterfaceID, matchedIfIndexesMap)
	} else {
		log.Tracef("[local interface resolution] expected 1 matching interface but found 0 (idType=%s, id=%s)", localInterfaceIDType, localInterfaceID)
	}
	return ""
}

func buildInterfaceIndexByIDType(interfaces []metadata.InterfaceMetadata) map[string]map[string][]int32 {
	interfaceIndexByIDType := make(map[string]map[string][]int32) // map[ID_TYPE]map[ID_VALUE]IF_INDEX
	for _, idType := range []string{"mac_address", "interface_name", "interface_alias", "interface_index"} {
		interfaceIndexByIDType[idType] = make(map[string][]int32)
	}
	for _, devInterface := range interfaces {
		interfaceIndexByIDType["mac_address"][devInterface.MacAddress] = append(interfaceIndexByIDType["mac_address"][devInterface.MacAddress], devInterface.Index)
		interfaceIndexByIDType["interface_name"][devInterface.Name] = append(interfaceIndexByIDType["interface_name"][devInterface.Name], devInterface.Index)
		interfaceIndexByIDType["interface_alias"][devInterface.Alias] = append(interfaceIndexByIDType["interface_alias"][devInterface.Alias], devInterface.Index)

		// interface_index is not a type defined by LLDP, it's used in local interface "smart resolution" when the idType is not present
		strIndex := strconv.Itoa(int(devInterface.Index))
		interfaceIndexByIDType["interface_index"][strIndex] = append(interfaceIndexByIDType["interface_index"][strIndex], devInterface.Index)
	}
	return interfaceIndexByIDType
}

func getRemManIPAddrByLLDPRemIndex(remManIndexes []string) map[string]string {
	remManAddrByRemIndex := make(map[string]string)
	for _, fullIndex := range remManIndexes {
		indexElems := strings.Split(fullIndex, ".")
		if len(indexElems) < 9 {
			// We expect the index to be at least 9 elements (IPv4)
			// 1 lldpRemTimeMark
			// 1 lldpRemLocalPortNum
			// 1 lldpRemIndex
			// 1 lldpRemManAddrSubtype (1 for IPv4, 2 for IPv6)
			// 5|17 lldpRemManAddr (4 for IPv4 and 17 for IPv6)
			//      the first elements is the IP type e.g. 4 for IPv4
			continue
		}
		lldpRemIndex := indexElems[2]
		lldpRemManAddrSubtype := indexElems[3]
		ipAddrType := indexElems[4]
		lldpRemManAddr := indexElems[5:]

		// We only support IPv4 for the moment
		// TODO: Support IPv6
		if lldpRemManAddrSubtype == "1" && ipAddrType == "4" {
			remManAddrByRemIndex[lldpRemIndex] = strings.Join(lldpRemManAddr, ".")
		}
	}
	return remManAddrByRemIndex
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

func batchPayloads(namespace string, subnet string, collectTime time.Time, batchSize int, device metadata.DeviceMetadata, interfaces []metadata.InterfaceMetadata, ipAddresses []metadata.IPAddressMetadata, topologyLinks []metadata.TopologyLinkMetadata) []metadata.NetworkDevicesMetadata {
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

	for _, ipAddress := range ipAddresses {
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
		payload.IPAddresses = append(payload.IPAddresses, ipAddress)
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
