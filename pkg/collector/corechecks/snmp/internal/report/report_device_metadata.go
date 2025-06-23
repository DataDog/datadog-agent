// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	json "encoding/json"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	sortutil "github.com/DataDog/datadog-agent/pkg/util/sort"

	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/profile/profiledefinition"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/utils"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/lldp"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/metadata"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

const interfaceStatusMetric = "snmp.interface.status"
const topologyLinkSourceTypeLLDP = "lldp"
const topologyLinkSourceTypeCDP = "cdp"
const ciscoNetworkProtocolIPv4 = "1"
const ciscoNetworkProtocolIPv6 = "20"
const inetAddressUnknown = "0"
const inetAddressIPv4 = "1"

var supportedDeviceTypes = map[string]bool{
	"access_point":  true,
	"firewall":      true,
	"load_balancer": true,
	"pdu":           true,
	"printer":       true,
	"router":        true,
	"sd-wan":        true,
	"sensor":        true,
	"server":        true,
	"storage":       true,
	"switch":        true,
	"ups":           true,
	"wlc":           true,
}

// ReportNetworkDeviceMetadata reports device metadata
func (ms *MetricSender) ReportNetworkDeviceMetadata(config *checkconfig.CheckConfig, profile profiledefinition.ProfileDefinition, store *valuestore.ResultValueStore, origTags []string, origMetricTags []string, collectTime time.Time, deviceStatus devicemetadata.DeviceStatus, pingStatus devicemetadata.DeviceStatus, diagnoses []devicemetadata.DiagnosisMetadata) {
	tags := utils.CopyStrings(origTags)
	tags = sortutil.UniqInPlace(tags)

	metricTags := utils.CopyStrings(origMetricTags)
	metricTags = sortutil.UniqInPlace(metricTags)

	metadataStore := buildMetadataStore(profile.Metadata, store)

	devices := []devicemetadata.DeviceMetadata{buildNetworkDeviceMetadata(config.DeviceID, config.DeviceIDTags, config, profile, metadataStore, tags, deviceStatus, pingStatus)}

	interfaces := buildNetworkInterfacesMetadata(config.DeviceID, metadataStore)
	ipAddresses := buildNetworkIPAddressesMetadata(config.DeviceID, metadataStore)
	topologyLinks := buildNetworkTopologyMetadata(config.DeviceID, metadataStore, interfaces)
	vpnTunnels := buildVPNTunnelsMetadata(config.DeviceID, metadataStore)

	metadataPayloads := devicemetadata.BatchPayloads(integrations.SNMP, config.Namespace, config.ResolvedSubnetName, collectTime, devicemetadata.PayloadMetadataBatchSize, devices, interfaces, ipAddresses, topologyLinks, vpnTunnels, nil, diagnoses)

	for _, payload := range metadataPayloads {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			log.Errorf("Error marshalling device metadata: %s", err)
			return
		}
		ms.sender.EventPlatformEvent(payloadBytes, eventplatform.EventTypeNetworkDevicesMetadata)
	}

	// Telemetry
	for _, interfaceStatus := range interfaces {
		status := string(computeInterfaceStatus(interfaceStatus.AdminStatus, interfaceStatus.OperStatus))
		interfaceIndex := strconv.Itoa(int(interfaceStatus.Index))
		interfaceTags := []string{
			"status:" + status,
			"admin_status:" + interfaceStatus.AdminStatus.AsString(),
			"oper_status:" + interfaceStatus.OperStatus.AsString(),
			"interface_index:" + interfaceIndex,
		}
		if interfaceStatus.Name != "" {
			interfaceTags = append(interfaceTags, "interface:"+interfaceStatus.Name)
		}
		if interfaceStatus.Alias != "" {
			interfaceTags = append(interfaceTags, "interface_alias:"+interfaceStatus.Alias)
		}
		interfaceTags = append(interfaceTags, metricTags...)

		// append user's custom interface tags
		interfaceCfg, err := getInterfaceConfig(ms.interfaceConfigs, interfaceIndex, interfaceTags)
		if err != nil {
			log.Tracef("unable to tag %s metric with interface_config data: %s", interfaceStatusMetric, err.Error())
		}
		interfaceTags = append(interfaceTags, interfaceCfg.Tags...)

		ms.sender.Gauge(interfaceStatusMetric, 1, ms.hostname, interfaceTags)
	}
}

func computeInterfaceStatus(adminStatus devicemetadata.IfAdminStatus, operStatus devicemetadata.IfOperStatus) devicemetadata.InterfaceStatus {
	if adminStatus == devicemetadata.AdminStatusUp {
		switch {
		case operStatus == devicemetadata.OperStatusUp:
			return devicemetadata.InterfaceStatusUp
		case operStatus == devicemetadata.OperStatusDown:
			return devicemetadata.InterfaceStatusDown
		}
		return devicemetadata.InterfaceStatusWarning
	}
	if adminStatus == devicemetadata.AdminStatusDown {
		switch {
		case operStatus == devicemetadata.OperStatusUp:
			return devicemetadata.InterfaceStatusDown
		case operStatus == devicemetadata.OperStatusDown:
			return devicemetadata.InterfaceStatusOff
		}
		return devicemetadata.InterfaceStatusWarning
	}
	if adminStatus == devicemetadata.AdminStatusTesting {
		switch {
		case operStatus != devicemetadata.OperStatusDown:
			return devicemetadata.InterfaceStatusWarning
		}
	}
	return devicemetadata.InterfaceStatusDown
}

func buildMetadataStore(metadataConfigs profiledefinition.MetadataConfig, values *valuestore.ResultValueStore) *metadata.Store {
	metadataStore := metadata.NewMetadataStore()
	if values == nil {
		return metadataStore
	}

	for resourceName, metadataConfig := range metadataConfigs {
		for fieldName, field := range metadataConfig.Fields {
			fieldFullName := resourceName + "." + fieldName

			var symbols []profiledefinition.SymbolConfig
			if field.Symbol.OID != "" {
				symbols = append(symbols, field.Symbol)
			}
			symbols = append(symbols, field.Symbols...)

			if profiledefinition.IsMetadataResourceWithScalarOids(resourceName) {
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

func buildNetworkDeviceMetadata(deviceID string, idTags []string, config *checkconfig.CheckConfig, profile profiledefinition.ProfileDefinition, store *metadata.Store, tags []string, deviceStatus devicemetadata.DeviceStatus, pingStatus devicemetadata.DeviceStatus) devicemetadata.DeviceMetadata {
	var vendor, sysName, sysDescr, sysObjectID, location, serialNumber, version, productName, model, osName, osVersion, osHostname, deviceType, profileName string
	var profileVersion uint64
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
		deviceType = getDeviceType(store)
	}

	profileName = profile.Name
	profileVersion = profile.Version
	if vendor == "" {
		vendor = profile.Device.Vendor
	}

	return devicemetadata.DeviceMetadata{
		ID:             deviceID,
		IDTags:         idTags,
		Name:           sysName,
		Description:    sysDescr,
		IPAddress:      config.IPAddress,
		SysObjectID:    sysObjectID,
		Location:       location,
		Profile:        profileName,
		ProfileVersion: profileVersion,
		Vendor:         vendor,
		Tags:           tags,
		Subnet:         config.ResolvedSubnetName,
		Status:         deviceStatus,
		PingStatus:     pingStatus,
		SerialNumber:   serialNumber,
		Version:        version,
		ProductName:    productName,
		Model:          model,
		OsName:         osName,
		OsVersion:      osVersion,
		OsHostname:     osHostname,
		DeviceType:     deviceType,
		Integration:    common.SnmpIntegrationName,
	}
}

func getDeviceType(store *metadata.Store) string {
	deviceType := strings.ToLower(store.GetScalarAsString("device.type"))
	if deviceType == "" {
		return "other"
	}
	_, isValidType := supportedDeviceTypes[deviceType]
	if isValidType {
		return deviceType
	}
	log.Warnf("Unsupported device type: %s", deviceType)
	return "other"
}

func buildNetworkInterfacesMetadata(deviceID string, store *metadata.Store) []devicemetadata.InterfaceMetadata {
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
	var interfaces []devicemetadata.InterfaceMetadata
	for _, strIndex := range indexes {
		index, err := strconv.ParseInt(strIndex, 10, 32)
		if err != nil {
			log.Warnf("interface metadata: invalid index: %d", index)
			continue
		}

		ifIDTags := store.GetIDTags("interface", strIndex)

		name := store.GetColumnAsString("interface.name", strIndex)
		networkInterface := devicemetadata.InterfaceMetadata{
			DeviceID:    deviceID,
			Index:       int32(index),
			Name:        name,
			Alias:       store.GetColumnAsString("interface.alias", strIndex),
			Description: store.GetColumnAsString("interface.description", strIndex),
			MacAddress:  store.GetColumnAsString("interface.mac_address", strIndex),
			AdminStatus: devicemetadata.IfAdminStatus((store.GetColumnAsFloat("interface.admin_status", strIndex))),
			OperStatus:  devicemetadata.IfOperStatus((store.GetColumnAsFloat("interface.oper_status", strIndex))),
			IDTags:      ifIDTags,
		}
		interfaces = append(interfaces, networkInterface)
	}
	return interfaces
}

func buildNetworkIPAddressesMetadata(deviceID string, store *metadata.Store) []devicemetadata.IPAddressMetadata {
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
	var ipAddresses []devicemetadata.IPAddressMetadata
	for _, strIndex := range indexes {
		index := store.GetColumnAsString("ip_addresses.if_index", strIndex)
		Netmask := store.GetColumnAsString("ip_addresses.netmask", strIndex)
		ipAddress := devicemetadata.IPAddressMetadata{
			InterfaceID: deviceID + ":" + index,
			IPAddress:   strIndex,
			Prefixlen:   int32(netmaskToPrefixlen(Netmask)),
		}
		ipAddresses = append(ipAddresses, ipAddress)
	}
	return ipAddresses
}

func buildNetworkTopologyMetadata(deviceID string, store *metadata.Store, interfaces []devicemetadata.InterfaceMetadata) []devicemetadata.TopologyLinkMetadata {
	if store == nil {
		// it's expected that the value store is nil if we can't reach the device
		// in that case, we just return a nil slice.
		return nil
	}

	links := buildNetworkTopologyMetadataWithLLDP(deviceID, store, interfaces)
	if len(links) == 0 {
		links = buildNetworkTopologyMetadataWithCDP(deviceID, store, interfaces)
	}
	return links
}

func buildNetworkTopologyMetadataWithLLDP(deviceID string, store *metadata.Store, interfaces []devicemetadata.InterfaceMetadata) []devicemetadata.TopologyLinkMetadata {
	interfaceIndexByIDType := buildInterfaceIndexByIDType(interfaces)

	remManAddrByLLDPRemIndex := getRemManIPAddrByLLDPRemIndex(store.GetColumnIndexes("lldp_remote_management.interface_id_type"))

	indexes := store.GetColumnIndexes("lldp_remote.interface_id") // using `lldp_remote.interface_id` to get indexes since it's expected to be always present
	if len(indexes) == 0 {
		log.Debugf("Unable to build links metadata: no lldp_remote indexes found")
		return nil
	}
	sort.Strings(indexes)
	var links []devicemetadata.TopologyLinkMetadata
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

		newLink := devicemetadata.TopologyLinkMetadata{
			ID:          deviceID + ":" + remEntryUniqueID,
			SourceType:  topologyLinkSourceTypeLLDP,
			Integration: common.SnmpIntegrationName,
			Remote: &devicemetadata.TopologyLinkSide{
				Device: &devicemetadata.TopologyLinkDevice{
					Name:        store.GetColumnAsString("lldp_remote.device_name", strIndex),
					Description: store.GetColumnAsString("lldp_remote.device_desc", strIndex),
					ID:          remoteDeviceID,
					IDType:      remoteDeviceIDType,
					IPAddress:   remManAddrByLLDPRemIndex[lldpRemIndex],
				},
				Interface: &devicemetadata.TopologyLinkInterface{
					ID:          remoteInterfaceID,
					IDType:      remoteInterfaceIDType,
					Description: store.GetColumnAsString("lldp_remote.interface_desc", strIndex),
				},
			},
			Local: &devicemetadata.TopologyLinkSide{
				Interface: &devicemetadata.TopologyLinkInterface{
					DDID:   resolvedLocalInterfaceID,
					ID:     localInterfaceID,
					IDType: localInterfaceIDType,
				},
				Device: &devicemetadata.TopologyLinkDevice{
					DDID: deviceID,
				},
			},
		}
		links = append(links, newLink)
	}
	return links
}

//nolint:revive // TODO(NDM) Fix revive linter
func buildNetworkTopologyMetadataWithCDP(deviceID string, store *metadata.Store, _ []devicemetadata.InterfaceMetadata) []devicemetadata.TopologyLinkMetadata {
	indexes := store.GetColumnIndexes("cdp_remote.interface_id") // using `cdp_remote.interface_id` to get indexes since it's expected to be always present
	if len(indexes) == 0 {
		log.Debugf("Unable to build links metadata: no cdp_remote indexes found")
		return nil
	}
	sort.Strings(indexes)
	var links []devicemetadata.TopologyLinkMetadata
	for _, strIndex := range indexes {
		indexElems := strings.Split(strIndex, ".")

		// The cdpCacheEntry index is composed of 2 elements separated by `.`: cdpCacheIfIndex, cdpCacheDeviceIndex
		if len(indexElems) != 2 {
			log.Debugf("Expected 2 index elements, but got %d, index=`%s`", len(indexElems), strIndex)
			continue
		}

		cdpCacheIfIndex := indexElems[0]
		cdpCacheDeviceIndex := indexElems[1]

		remoteDeviceAddress := getRemDeviceAddressByCDPRemIndex(store, strIndex)

		resolvedLocalInterfaceID := deviceID + ":" + cdpCacheIfIndex

		// remEntryUniqueID: The combination of cdpCacheIfIndex and cdpCacheDeviceIndex is expected to be unique for each entry in cdpCacheTable
		remEntryUniqueID := cdpCacheIfIndex + "." + cdpCacheDeviceIndex

		newLink := devicemetadata.TopologyLinkMetadata{
			ID:          deviceID + ":" + remEntryUniqueID,
			SourceType:  topologyLinkSourceTypeCDP,
			Integration: common.SnmpIntegrationName,
			Remote: &devicemetadata.TopologyLinkSide{
				Device: &devicemetadata.TopologyLinkDevice{
					Name:        store.GetColumnAsString("cdp_remote.device_name", strIndex),
					Description: store.GetColumnAsString("cdp_remote.device_desc", strIndex),
					ID:          store.GetColumnAsString("cdp_remote.device_id", strIndex),
					IDType:      "",
					IPAddress:   remoteDeviceAddress,
				},
				Interface: &devicemetadata.TopologyLinkInterface{
					ID:          store.GetColumnAsString("cdp_remote.interface_id", strIndex),
					IDType:      devicemetadata.IDTypeInterfaceName,
					Description: "",
				},
			},
			Local: &devicemetadata.TopologyLinkSide{
				Interface: &devicemetadata.TopologyLinkInterface{
					DDID:   resolvedLocalInterfaceID,
					ID:     "",
					IDType: "",
				},
				Device: &devicemetadata.TopologyLinkDevice{
					DDID: deviceID,
				},
			},
		}
		links = append(links, newLink)
	}
	return links
}

func getRemDeviceAddressByCDPRemIndex(store *metadata.Store, strIndex string) string {
	remoteDeviceAddress := getRemDeviceAddressIfIPType(store, strIndex, "device_address_type", "device_address")
	if remoteDeviceAddress != "" {
		return remoteDeviceAddress
	}

	remoteDeviceSecondaryAddress := getRemDeviceAddressIfIPType(store, strIndex, "device_secondary_address_type", "device_secondary_address")
	if remoteDeviceSecondaryAddress != "" {
		return remoteDeviceSecondaryAddress
	}

	// Note: If this also returns an empty string, this won't pass the backend check and will generate the error
	// "deviceIP cannot be empty (except when interface id_type is mac_address)"
	return getRemDeviceAddressIfIPType(store, strIndex, "device_cache_address_type", "device_cache_address")
}

func getRemDeviceAddressIfIPType(store *metadata.Store, strIndex string, addressTypeField string, addressField string) string {
	remoteDeviceAddressType := store.GetColumnAsString("cdp_remote."+addressTypeField, strIndex)
	if remoteDeviceAddressType == ciscoNetworkProtocolIPv4 || remoteDeviceAddressType == ciscoNetworkProtocolIPv6 {
		return net.IP(store.GetColumnAsByteArray("cdp_remote."+addressField, strIndex)).String()
	}
	return ""
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

func buildInterfaceIndexByIDType(interfaces []devicemetadata.InterfaceMetadata) map[string]map[string][]int32 {
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
	if idType == devicemetadata.IDTypeMacAddress {
		remoteDeviceID = formatColonSepBytes(store.GetColumnAsByteArray(field, strIndex))
	} else {
		remoteDeviceID = store.GetColumnAsString(field, strIndex)
	}
	return remoteDeviceID
}

func buildVPNTunnelsMetadata(deviceID string, store *metadata.Store) []devicemetadata.VPNTunnelMetadata {
	if store == nil {
		// it's expected that the value store is nil if we can't reach the device
		// in that case, we just return a nil slice.
		return nil
	}

	vpnTunnelIndexes := store.GetColumnIndexes("cisco_ipsec_tunnel.local_outside_ip")
	if len(vpnTunnelIndexes) == 0 {
		log.Debugf("Unable to build VPN tunnels metadata: no cisco_ipsec_tunnel.local_outside_ip found")
		return nil
	}
	sort.Strings(vpnTunnelIndexes)

	vpnTunnelStore := NewVPNTunnelStore()

	for _, strIndex := range vpnTunnelIndexes {
		indexElems := strings.Split(strIndex, ".")
		if len(indexElems) != 1 {
			// The cipSecTunnelEntry index is composed of 1 element: cipSecTunIndex
			log.Debugf("Expected 1 index element in cipSecTunnelEntry, but got %d, index=`%s`", len(indexElems), strIndex)
			continue
		}

		localOutsideIP := net.IP(store.GetColumnAsByteArray("cisco_ipsec_tunnel.local_outside_ip", strIndex)).String()
		remoteOutsideIP := net.IP(store.GetColumnAsByteArray("cisco_ipsec_tunnel.remote_outside_ip", strIndex)).String()

		vpnTunnelStore.AddTunnel(devicemetadata.VPNTunnelMetadata{
			DeviceID:        deviceID,
			LocalOutsideIP:  localOutsideIP,
			RemoteOutsideIP: remoteOutsideIP,
			Protocol:        "ipsec",
		})
	}

	resolveVPNTunnelsRoutes(store, vpnTunnelStore)

	return vpnTunnelStore.ToNormalizedSortedSlice()
}

func resolveVPNTunnelsRoutes(store *metadata.Store, vpnTunnelStore VPNTunnelStore) {
	routeDeprecatedIndexes := store.GetColumnIndexes("ipforward_deprecated.if_index")
	routeIndexes := store.GetColumnIndexes("ipforward.if_index")
	if len(routeDeprecatedIndexes) == 0 && len(routeIndexes) == 0 {
		return
	}
	sort.Strings(routeDeprecatedIndexes)
	sort.Strings(routeIndexes)

	routeSet := make(map[DeviceRoute]struct{})
	routesByIfIndex := make(RoutesByIfIndex)

	for _, strIndex := range routeDeprecatedIndexes {
		routeStatus := store.GetColumnAsString("ipforward_deprecated.route_status", strIndex)
		if routeStatus != "1" {
			continue
		}

		indexElems := strings.Split(strIndex, ".")
		if len(indexElems) != 13 {
			// We expect the index to be 13 elements:
			// 4 ipCidrRouteDest
			// 4 ipCidrRouteMask
			// 1 ipCidrRouteTos
			// 4 ipCidrRouteNextHop
			log.Debugf("Expected 13 index element in ipCidrRouteEntry, but got %d, index=`%s`", len(indexElems), strIndex)
			continue
		}

		routeDestination := strings.Join(indexElems[0:4], ".")
		routePrefixLen := netmaskToPrefixlen(strings.Join(indexElems[4:8], "."))
		nextHopIP := strings.Join(indexElems[9:13], ".")

		ifIndex := store.GetColumnAsString("ipforward_deprecated.if_index", strIndex)

		route := DeviceRoute{
			Destination: routeDestination,
			PrefixLen:   routePrefixLen,
			NextHopIP:   nextHopIP,
			IfIndex:     ifIndex,
		}
		if _, exists := routeSet[route]; exists {
			continue
		}

		routeSet[route] = struct{}{}
		routesByIfIndex[ifIndex] = append(routesByIfIndex[ifIndex], route)

		resolveRouteByNextHop(route, vpnTunnelStore)
	}

	for _, strIndex := range routeIndexes {
		routeStatus := store.GetColumnAsString("ipforward.route_status", strIndex)
		if routeStatus != "1" {
			continue
		}

		/* Example with full OID: 1.3.6.1.2.1.4.24.7.1.7.2.16.255.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.8.2.0.0.0.0
		.1.3.6.1.2.1.4.24.7.1.7: Base OID
		.2: Destination type
		.16: Destination IP length
		.255.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0: Destination IP
		.8: Prefix
		.2: Policy length
		.0.0: Policy
		.0: Next hop type
		.0: Next hop length
		*/
		indexElems := strings.Split(strIndex, ".")
		currMaxIndex := 2
		if len(indexElems) < currMaxIndex {
			continue
		}

		destAddrType := indexElems[currMaxIndex-2]
		if destAddrType != inetAddressIPv4 {
			continue
		}

		destLength, err := strconv.Atoi(indexElems[currMaxIndex-1])
		if err != nil {
			continue
		}

		currMaxIndex += destLength
		if len(indexElems) < currMaxIndex {
			continue
		}

		routeDestination := strings.Join(indexElems[currMaxIndex-destLength:currMaxIndex], ".")

		currMaxIndex += 2
		if len(indexElems) < currMaxIndex {
			continue
		}

		routePrefixLen, err := strconv.Atoi(indexElems[currMaxIndex-2])
		if err != nil {
			continue
		}

		policyLength, err := strconv.Atoi(indexElems[currMaxIndex-1])
		if err != nil {
			continue
		}

		currMaxIndex += policyLength + 2
		if len(indexElems) < currMaxIndex {
			continue
		}

		nextHopAddrType := indexElems[currMaxIndex-2]
		if nextHopAddrType != inetAddressUnknown && nextHopAddrType != inetAddressIPv4 {
			continue
		}

		nextHopLength, err := strconv.Atoi(indexElems[currMaxIndex-1])
		if err != nil {
			continue
		}

		currMaxIndex += nextHopLength
		if len(indexElems) < currMaxIndex {
			continue
		}

		nextHopIP := "0.0.0.0"
		if nextHopLength != 0 {
			nextHopIP = strings.Join(indexElems[currMaxIndex-nextHopLength:currMaxIndex], ".")
		}

		ifIndex := store.GetColumnAsString("ipforward.if_index", strIndex)

		route := DeviceRoute{
			Destination: routeDestination,
			PrefixLen:   routePrefixLen,
			NextHopIP:   nextHopIP,
			IfIndex:     ifIndex,
		}
		if _, exists := routeSet[route]; exists {
			continue
		}

		routeSet[route] = struct{}{}
		routesByIfIndex[ifIndex] = append(routesByIfIndex[ifIndex], route)

		resolveRouteByNextHop(route, vpnTunnelStore)
	}

	for _, vpnTunnel := range vpnTunnelStore.ByOutsideIPs {
		if len(vpnTunnel.RouteAddresses) == 0 {
			resolveRoutesByIfIndex(store, vpnTunnelStore, routesByIfIndex)
		}
	}
}

func resolveRouteByNextHop(route DeviceRoute, vpnTunnelStore VPNTunnelStore) {
	vpnTunnels, exists := vpnTunnelStore.GetTunnelsByRemoteOutsideIP(route.NextHopIP)
	if !exists {
		return
	}

	for _, vpnTunnel := range vpnTunnels {
		vpnTunnel.RouteAddresses = append(vpnTunnel.RouteAddresses,
			fmt.Sprintf("%s/%d", route.Destination, route.PrefixLen))
	}
}

func resolveRoutesByIfIndex(store *metadata.Store, vpnTunnelStore VPNTunnelStore, routesByIfIndex RoutesByIfIndex) {
	tunnelDeprecatedIndexes := store.GetColumnIndexes("tunnel_config_deprecated.if_index")
	tunnelIndexes := store.GetColumnIndexes("tunnel_config.if_index")
	if len(tunnelDeprecatedIndexes) == 0 && len(tunnelIndexes) == 0 {
		return
	}
	sort.Strings(tunnelDeprecatedIndexes)
	sort.Strings(tunnelIndexes)

	tunnelSet := make(map[DeviceTunnel]struct{})

	for _, strIndex := range tunnelDeprecatedIndexes {
		indexElems := strings.Split(strIndex, ".")
		if len(indexElems) != 10 {
			// We expect the index to be 10 elements:
			// 4 tunnelConfigLocalAddress
			// 4 tunnelConfigRemoteAddress
			// 1 tunnelConfigEncapsMethod
			// 1 tunnelConfigID
			log.Debugf("Expected 10 index element in tunnelConfigEntry, but got %d, index=`%s`", len(indexElems), strIndex)
			continue
		}

		localIP := strings.Join(indexElems[0:4], ".")
		remoteIP := strings.Join(indexElems[4:8], ".")

		ifIndex := store.GetColumnAsString("tunnel_config_deprecated.if_index", strIndex)

		tunnel := DeviceTunnel{
			LocalIP:  localIP,
			RemoteIP: remoteIP,
			IfIndex:  ifIndex,
		}
		if _, exists := tunnelSet[tunnel]; exists {
			continue
		}

		tunnelSet[tunnel] = struct{}{}

		addRoutesByIfIndexToVPNTunnel(tunnel, vpnTunnelStore, routesByIfIndex)
	}

	for _, strIndex := range tunnelIndexes {
		/* Example with full OID: .1.3.6.1.2.1.10.131.1.1.3.1.6.1.4.10.0.2.91.4.3.134.54.211.1.1
		.1.3.6.1.2.1.10.131.1.1.3.1.6: Base OID
		.1: Addresses type
		.4: Local address IP length
		.10.0.2.91: Local address IP
		.4: Remote address IP length
		.3.134.54.211: Remote address IP
		.1: Tunnel encapsulation method
		.1: Tunnel config ID
		*/
		indexElems := strings.Split(strIndex, ".")
		currMaxIndex := 2
		if len(indexElems) < currMaxIndex {
			continue
		}

		addrType := indexElems[currMaxIndex-2]
		if addrType != inetAddressIPv4 {
			continue
		}

		localAddrLength, err := strconv.Atoi(indexElems[currMaxIndex-1])
		if err != nil {
			continue
		}

		currMaxIndex += localAddrLength
		if len(indexElems) < currMaxIndex {
			continue
		}

		localIP := strings.Join(indexElems[currMaxIndex-localAddrLength:currMaxIndex], ".")

		currMaxIndex++
		if len(indexElems) < currMaxIndex {
			continue
		}

		remoteAddrLength, err := strconv.Atoi(indexElems[currMaxIndex-1])
		if err != nil {
			continue
		}

		currMaxIndex += remoteAddrLength
		if len(indexElems) < currMaxIndex {
			continue
		}

		remoteIP := strings.Join(indexElems[currMaxIndex-remoteAddrLength:currMaxIndex], ".")

		ifIndex := store.GetColumnAsString("tunnel_config.if_index", strIndex)

		tunnel := DeviceTunnel{
			LocalIP:  localIP,
			RemoteIP: remoteIP,
			IfIndex:  ifIndex,
		}
		if _, exists := tunnelSet[tunnel]; exists {
			continue
		}

		tunnelSet[tunnel] = struct{}{}

		addRoutesByIfIndexToVPNTunnel(tunnel, vpnTunnelStore, routesByIfIndex)
	}
}

func addRoutesByIfIndexToVPNTunnel(tunnel DeviceTunnel, vpnTunnelStore VPNTunnelStore, routesByIfIndex RoutesByIfIndex) {
	vpnTunnel, exists := vpnTunnelStore.GetTunnelByOutsideIPs(tunnel.LocalIP, tunnel.RemoteIP)
	if !exists {
		return
	}

	vpnTunnel.InterfaceID = vpnTunnel.DeviceID + ":" + tunnel.IfIndex

	routes, exists := routesByIfIndex[tunnel.IfIndex]
	if !exists {
		return
	}

	for _, route := range routes {
		vpnTunnel.RouteAddresses = append(vpnTunnel.RouteAddresses,
			fmt.Sprintf("%s/%d", route.Destination, route.PrefixLen))
	}
}
