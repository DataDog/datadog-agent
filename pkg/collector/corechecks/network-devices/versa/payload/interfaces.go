// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package payload

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"go.uber.org/multierr"
)

// GetInterfaceMetadata processes interface API payloads to create interface metadata
func GetInterfaceMetadata(namespace string, deviceNameToIPMap map[string]string, ifaces []client.Interface) ([]devicemetadata.InterfaceMetadata, error) {
	interfaceMetadata := make([]devicemetadata.InterfaceMetadata, 0, len(ifaces))
	var combinedErrs error
	for _, iface := range ifaces {
		md, err := buildInterfaceMetadata(namespace, deviceNameToIPMap, iface)
		if err != nil {
			combinedErrs = multierr.Append(combinedErrs, err)
			continue
		}
		interfaceMetadata = append(interfaceMetadata, md)
	}

	return interfaceMetadata, combinedErrs
}

func buildInterfaceMetadata(namespace string, deviceNameToIPMap map[string]string, iface client.Interface) (devicemetadata.InterfaceMetadata, error) {
	deviceIP, ok := deviceNameToIPMap[iface.DeviceName]
	if !ok {
		return devicemetadata.InterfaceMetadata{}, fmt.Errorf("couldn't find matching device %q for interface %q", iface.DeviceName, iface.Name)
	}

	id := buildDeviceID(namespace, deviceIP)

	return devicemetadata.InterfaceMetadata{
		DeviceID:    id,
		IDTags:      []string{"interface:" + iface.Name},
		RawID:       iface.Name,
		RawIDType:   "versa_interface",
		Name:        iface.Name,
		MacAddress:  iface.MAC,
		AdminStatus: adminStatus(iface.IfAdminStatus),
		OperStatus:  operStatus(iface.IfOperStatus),
	}, nil
}

func adminStatus(adminStatus string) devicemetadata.IfAdminStatus {
	adminStatusMap := map[string]devicemetadata.IfAdminStatus{
		"up":      devicemetadata.AdminStatusUp,
		"down":    devicemetadata.AdminStatusDown,
		"testing": devicemetadata.AdminStatusTesting,
	}

	status, ok := adminStatusMap[adminStatus]
	if !ok {
		return devicemetadata.AdminStatusDown
	}

	return status
}

func operStatus(operStatus string) devicemetadata.IfOperStatus {
	operStatusMap := map[string]devicemetadata.IfOperStatus{
		"up":               devicemetadata.OperStatusUp,
		"down":             devicemetadata.OperStatusDown,
		"testing":          devicemetadata.OperStatusTesting,
		"unknown":          devicemetadata.OperStatusUnknown,
		"dormant":          devicemetadata.OperStatusDormant,
		"not_present":      devicemetadata.OperStatusNotPresent,
		"lower_layer_down": devicemetadata.OperStatusLowerLayerDown,
	}

	status, ok := operStatusMap[operStatus]
	if !ok {
		return devicemetadata.OperStatusDown
	}

	return status
}
