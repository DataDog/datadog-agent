// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package report

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"strconv"
	"time"
)

var operStatusMap = map[string]devicemetadata.IfOperStatus{
	"Up":   devicemetadata.OperStatusUp,
	"Down": devicemetadata.OperStatusDown,
}

var adminStatusMap = map[string]devicemetadata.IfAdminStatus{
	"Up":   devicemetadata.AdminStatusUp,
	"Down": devicemetadata.AdminStatusDown,
}

var cEdgeOperStatusMap = map[string]devicemetadata.IfOperStatus{
	"if-oper-state-ready": devicemetadata.OperStatusUp,
	"if-oper-state-down":  devicemetadata.OperStatusDown,
}

var cEdgeAdminStatusMap = map[string]devicemetadata.IfAdminStatus{
	"if-state-up":   devicemetadata.AdminStatusUp,
	"if-state-down": devicemetadata.AdminStatusDown,
}

// SendMetadata send Cisco SD-WAN device and interface metadata
func (ms *SDWanSender) SendMetadata(devices []client.Device, vEdgeInterfaces []client.InterfaceState, cEdgeInterfaces []client.CEdgeInterfaceState) {
	var devicesMetadata []devicemetadata.DeviceMetadata
	for _, device := range devices {
		devicesMetadata = append(devicesMetadata, buildDeviceMetadata(device))
	}

	var interfacesMetadata []devicemetadata.InterfaceMetadata
	for _, itf := range vEdgeInterfaces {
		interfacesMetadata = append(interfacesMetadata, buildVEdgeInterfaceMetadata(itf))
	}

	for _, itf := range cEdgeInterfaces {
		metadata, err := buildCEdgeInterfaceMetadata(itf)
		if err != nil {
			continue
		}
		interfacesMetadata = append(interfacesMetadata, *metadata)
	}

	collectionTime := time.Now()
	metadataPayloads := devicemetadata.BatchPayloads("thibaud", "", collectionTime, devicemetadata.PayloadMetadataBatchSize, devicesMetadata, interfacesMetadata, nil, nil, nil, nil)
	for _, payload := range metadataPayloads {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			// TODO
			continue
		}
		ms.sender.EventPlatformEvent(payloadBytes, eventplatform.EventTypeNetworkDevicesMetadata)
	}
}

func buildDeviceMetadata(device client.Device) devicemetadata.DeviceMetadata {
	status := devicemetadata.DeviceStatusUnreachable
	if device.Reachability == "reachable" {
		status = devicemetadata.DeviceStatusReachable
	}

	id := fmt.Sprintf("sdwan:%s", device.SystemIP)
	return devicemetadata.DeviceMetadata{
		ID:           id,
		IPAddress:    device.SystemIP,
		Vendor:       "cisco",
		Name:         device.HostName,
		Tags:         []string{"site_id:" + device.SiteID, "test:thibaud"},
		IDTags:       []string{"device_name:" + device.SystemIP},
		Status:       status,
		Model:        device.DeviceModel,
		OsName:       device.DeviceOs,
		Version:      device.Version,
		SerialNumber: device.BoardSerial,
		Integration:  "cisco-sdwan",
	}
}

// FIXME : This will create duplicate interfaces (ipv6 and ipv4) sending twice the payloads for nothing
func buildVEdgeInterfaceMetadata(itf client.InterfaceState) devicemetadata.InterfaceMetadata {
	return devicemetadata.InterfaceMetadata{
		DeviceID:    fmt.Sprintf("sdwan:%s", itf.VmanageSystemIP),
		IDTags:      []string{fmt.Sprintf("interface:%s", itf.Ifname)},
		Index:       int32(itf.Ifindex),
		Name:        itf.Ifname,
		Description: itf.Desc,
		MacAddress:  itf.Hwaddr,
		AdminStatus: adminStatusMap[itf.IfAdminStatus],
		OperStatus:  operStatusMap[itf.IfOperStatus],
	}
}

func buildCEdgeInterfaceMetadata(itf client.CEdgeInterfaceState) (*devicemetadata.InterfaceMetadata, error) {
	index, err := strconv.Atoi(itf.Ifindex)
	if err != nil {
		return nil, err
	}

	return &devicemetadata.InterfaceMetadata{
		DeviceID:    fmt.Sprintf("sdwan:%s", itf.VmanageSystemIP),
		IDTags:      []string{fmt.Sprintf("interface:%s", itf.Ifname)},
		Index:       int32(index),
		Name:        itf.Ifname,
		Description: itf.Description,
		MacAddress:  itf.Hwaddr,
		AdminStatus: cEdgeAdminStatusMap[itf.IfAdminStatus],
		OperStatus:  cEdgeOperStatusMap[itf.IfOperStatus],
	}, nil
}
