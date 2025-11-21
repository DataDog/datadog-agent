// Package payload implement processing of Versa api responses
package payload

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

// GetDeviceMetadataFromAppliances process devices API payloads to build metadata
func GetTopologyMetadata() ([]devicemetadata.TopologyLinkMetadata, error) {
	log.Tracef("GetTopologyMetadata called")
	return []devicemetadata.TopologyLinkMetadata{}, nil
}


// func buildNetworkTopologyMetadataWithLLDP(deviceID string, store *metadata.Store, interfaces []devicemetadata.InterfaceMetadata) []devicemetadata.TopologyLinkMetadata {
// 	interfaceIndexByIDType := buildInterfaceIndexByIDType(interfaces)

// 	remManAddrByLLDPRemIndex := getRemManIPAddrByLLDPRemIndex(store.GetColumnIndexes("lldp_remote_management.interface_id_type"))

// 	indexes := store.GetColumnIndexes("lldp_remote.interface_id") // using `lldp_remote.interface_id` to get indexes since it's expected to be always present
// 	if len(indexes) == 0 {
// 		log.Debugf("Unable to build links metadata: no lldp_remote indexes found")
// 		return nil
// 	}
// 	sort.Strings(indexes)
// 	var links []devicemetadata.TopologyLinkMetadata
// 	for _, strIndex := range indexes {
// 		indexElems := strings.Split(strIndex, ".")

// 		// The lldpRemEntry index is composed of those 3 elements separate by `.`: lldpRemTimeMark, lldpRemLocalPortNum, lldpRemIndex
// 		if len(indexElems) != 3 {
// 			log.Debugf("Expected 3 index elements, but got %d, index=`%s`", len(indexElems), strIndex)
// 			continue
// 		}
// 		// TODO: Handle TimeMark at indexElems[0] if needed later
// 		//       See https://www.rfc-editor.org/rfc/rfc2021

// 		localPortNum := indexElems[1]
// 		lldpRemIndex := indexElems[2]

// 		remoteDeviceIDType := lldp.ChassisIDSubtypeMap[store.GetColumnAsString("lldp_remote.chassis_id_type", strIndex)]
// 		remoteDeviceID := formatID(remoteDeviceIDType, store, "lldp_remote.chassis_id", strIndex)

// 		remoteInterfaceIDType := lldp.PortIDSubTypeMap[store.GetColumnAsString("lldp_remote.interface_id_type", strIndex)]
// 		remoteInterfaceID := formatID(remoteInterfaceIDType, store, "lldp_remote.interface_id", strIndex)

// 		localInterfaceIDType := lldp.PortIDSubTypeMap[store.GetColumnAsString("lldp_local.interface_id_type", localPortNum)]
// 		localInterfaceID := formatID(localInterfaceIDType, store, "lldp_local.interface_id", localPortNum)

// 		resolvedLocalInterfaceID := resolveLocalInterface(deviceID, interfaceIndexByIDType, localInterfaceIDType, localInterfaceID)

// 		// remEntryUniqueID: The combination of localPortNum and lldpRemIndex is expected to be unique for each entry in
// 		//                   lldpRemTable. We don't include lldpRemTimeMark (used for filtering only recent data) since it can change often.
// 		remEntryUniqueID := localPortNum + "." + lldpRemIndex

// 		newLink := devicemetadata.TopologyLinkMetadata{
// 			ID:          deviceID + ":" + remEntryUniqueID,
// 			SourceType:  topologyLinkSourceTypeLLDP,
// 			Integration: common.SnmpIntegrationName,
// 			Remote: &devicemetadata.TopologyLinkSide{
// 				Device: &devicemetadata.TopologyLinkDevice{
// 					Name:        store.GetColumnAsString("lldp_remote.device_name", strIndex),
// 					Description: store.GetColumnAsString("lldp_remote.device_desc", strIndex),
// 					ID:          remoteDeviceID,
// 					IDType:      remoteDeviceIDType,
// 					IPAddress:   remManAddrByLLDPRemIndex[lldpRemIndex],
// 				},
// 				Interface: &devicemetadata.TopologyLinkInterface{
// 					ID:          remoteInterfaceID,
// 					IDType:      remoteInterfaceIDType,
// 					Description: store.GetColumnAsString("lldp_remote.interface_desc", strIndex),
// 				},
// 			},
// 			Local: &devicemetadata.TopologyLinkSide{
// 				Interface: &devicemetadata.TopologyLinkInterface{
// 					DDID:   resolvedLocalInterfaceID,
// 					ID:     localInterfaceID,
// 					IDType: localInterfaceIDType,
// 				},
// 				Device: &devicemetadata.TopologyLinkDevice{
// 					DDID: deviceID,
// 				},
// 			},
// 		}
// 		links = append(links, newLink)
// 	}
// 	return links
// }
