// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package payload

import (
	"fmt"
	"net"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/cisco-sdwan/client"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// https://github.com/YangModels/yang/blob/9442dda17a9a5f1f0db548512446e3d9ca37a955/vendor/cisco/xe/17131/Cisco-IOS-XE-interfaces-oper.yang#L305
var cEdgeOperStatusMap = map[string]devicemetadata.IfOperStatus{
	"if-oper-state-down":             devicemetadata.OperStatusDown,
	"if-oper-state-invalid":          devicemetadata.OperStatusDown,
	"if-oper-state-ready":            devicemetadata.OperStatusUp,
	"if-oper-state-no-pass":          devicemetadata.OperStatusDown,
	"if-oper-state-test":             devicemetadata.OperStatusTesting,
	"if-oper-state-unknown":          devicemetadata.OperStatusUnknown,
	"if-oper-state-dormant":          devicemetadata.OperStatusDormant,
	"if-oper-state-not-present":      devicemetadata.OperStatusNotPresent,
	"if-oper-state-lower-layer-down": devicemetadata.OperStatusLowerLayerDown,
}

// https://github.com/YangModels/yang/blob/9442dda17a9a5f1f0db548512446e3d9ca37a955/vendor/cisco/xe/17131/Cisco-IOS-XE-interfaces-oper.yang#L212
var cEdgeAdminStatusMap = map[string]devicemetadata.IfAdminStatus{
	"if-state-unknown": devicemetadata.AdminStatusDown,
	"if-state-up":      devicemetadata.AdminStatusUp,
	"if-state-down":    devicemetadata.AdminStatusDown,
	"if-state-test":    devicemetadata.AdminStatusTesting,
}

// CEdgeInterface is an implementation of CiscoInterface for cEdge devices
type CEdgeInterface struct {
	client.CEdgeInterfaceState
}

// ID returns an unique interface ID
func (itf *CEdgeInterface) ID() string {
	return fmt.Sprintf("%s:%s", itf.VmanageSystemIP, itf.Ifname)
}

// Index returns the interface index
func (itf *CEdgeInterface) Index() int {
	index, err := strconv.Atoi(itf.Ifindex)
	if err != nil {
		log.Warnf("Unable to parse cEdge interface %s index %s", itf.Ifname, itf.Ifindex)
	}
	return index
}

// Speed returns the interface speed
func (itf *CEdgeInterface) Speed() int {
	speed, err := strconv.Atoi(itf.SpeedMbps)
	if err != nil {
		log.Warnf("Unable to parse cEdge interface %s speed %s", itf.Ifname, itf.SpeedMbps)
	}
	return speed
}

// OperStatus returns the interface oper status
func (itf *CEdgeInterface) OperStatus() devicemetadata.IfOperStatus {
	return convertOperStatus(cEdgeOperStatusMap, itf.IfOperStatus)
}

// AdminStatus returns the interface admin status
func (itf *CEdgeInterface) AdminStatus() devicemetadata.IfAdminStatus {
	return convertAdminStatus(cEdgeAdminStatusMap, itf.IfAdminStatus)
}

// Metadata returns the interface metadata
func (itf *CEdgeInterface) Metadata(namespace string) devicemetadata.InterfaceMetadata {
	index, err := strconv.Atoi(itf.Ifindex)
	if err != nil {
		log.Warnf("Unable to parse cEdge interface %s index %s", itf.Ifname, itf.Ifindex)
	}

	return devicemetadata.InterfaceMetadata{
		DeviceID:    fmt.Sprintf("%s:%s", namespace, itf.VmanageSystemIP),
		IDTags:      []string{fmt.Sprintf("interface:%s", itf.Ifname)},
		Index:       int32(index),
		Name:        itf.Ifname,
		Description: itf.Description,
		MacAddress:  itf.Hwaddr,
		OperStatus:  convertOperStatus(cEdgeOperStatusMap, itf.IfOperStatus),
		AdminStatus: convertAdminStatus(cEdgeAdminStatusMap, itf.IfAdminStatus),
	}
}

// IPAddressMetadata returns the metadata for this interface's IP addresses
func (itf *CEdgeInterface) IPAddressMetadata(namespace string) (devicemetadata.IPAddressMetadata, error) {
	ip, prefiLen, err := parseIPFromCEdgeInterface(itf.IPAddress, itf.Ipv4SubnetMask)
	if err != nil {
		log.Warnf("Unable to parse cEdge interface %s IP %s", itf.Ifname, itf.IPAddress)
		return devicemetadata.IPAddressMetadata{}, err
	}

	return devicemetadata.IPAddressMetadata{
		InterfaceID: fmt.Sprintf("%s:%s:%s", namespace, itf.VmanageSystemIP, itf.Ifindex),
		IPAddress:   ip,
		Prefixlen:   prefiLen,
	}, nil
}

func parseIPFromCEdgeInterface(ip string, mask string) (string, int32, error) {
	ipaddr := net.ParseIP(ip)
	if ipaddr == nil {
		return "", 0, fmt.Errorf("invalid ip address")
	}

	ipMask := net.ParseIP(mask)
	if ipMask == nil {
		return "", 0, fmt.Errorf("invalid mask")
	}

	parsedMask := net.IPMask(ipMask.To4())
	prefixLen, _ := parsedMask.Size()

	return ipaddr.String(), int32(prefixLen), nil
}
