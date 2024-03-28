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
	// VmanageSystemIP is the device's System IP from vManage
	return fmt.Sprintf("%s:%s", itf.VmanageSystemIP, itf.Ifname)
}

// Index returns the interface index
func (itf *CEdgeInterface) Index() (int, error) {
	index, err := strconv.Atoi(itf.Ifindex)
	if err != nil {
		return 0, err
	}
	return index, nil
}

// GetSpeedMbps returns the interface speed
func (itf *CEdgeInterface) GetSpeedMbps() int {
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
func (itf *CEdgeInterface) Metadata(namespace string) (devicemetadata.InterfaceMetadata, error) {
	index, err := itf.Index()
	if err != nil {
		return devicemetadata.InterfaceMetadata{}, err
	}

	return devicemetadata.InterfaceMetadata{
		DeviceID:    fmt.Sprintf("%s:%s", namespace, itf.VmanageSystemIP), // VmanageSystemIP is the device's System IP from vManage
		IDTags:      []string{fmt.Sprintf("interface:%s", itf.Ifname)},
		Index:       int32(index),
		Name:        itf.Ifname,
		Description: itf.Description,
		MacAddress:  itf.Hwaddr,
		OperStatus:  convertOperStatus(cEdgeOperStatusMap, itf.IfOperStatus),
		AdminStatus: convertAdminStatus(cEdgeAdminStatusMap, itf.IfAdminStatus),
	}, nil
}

// IPV4AddressMetadata returns the metadata for this interface's IPV4 addresses
func (itf *CEdgeInterface) IPV4AddressMetadata(namespace string) (*devicemetadata.IPAddressMetadata, error) {
	if isEmptyCEdgeIP(itf.IPAddress) {
		return nil, nil
	}

	index, err := itf.Index()
	if err != nil {
		return nil, err
	}

	ip, err := parseCEdgeIP(itf.IPAddress)
	if err != nil {
		return nil, err
	}

	prefixLen, err := parseMask(itf.Ipv4SubnetMask)
	if err != nil {
		return nil, err
	}

	return &devicemetadata.IPAddressMetadata{
		InterfaceID: fmt.Sprintf("%s:%s:%d", namespace, itf.VmanageSystemIP, index),
		IPAddress:   ip,
		Prefixlen:   prefixLen,
	}, nil
}

// IPV6AddressMetadata returns the metadata for this interface's IPV6 addresses
func (itf *CEdgeInterface) IPV6AddressMetadata(namespace string) (*devicemetadata.IPAddressMetadata, error) {
	if isEmptyCEdgeIP(itf.IPV6Address) {
		return nil, nil
	}

	index, err := itf.Index()
	if err != nil {
		return nil, err
	}

	ip, err := parseCEdgeIP(itf.IPV6Address)
	if err != nil {
		return nil, err
	}

	return &devicemetadata.IPAddressMetadata{
		InterfaceID: fmt.Sprintf("%s:%s:%d", namespace, itf.VmanageSystemIP, index), // VmanageSystemIP is the device's System IP from vManage
		IPAddress:   ip,
	}, nil
}

func isEmptyCEdgeIP(ip string) bool {
	return ip == ""
}

func parseCEdgeIP(ip string) (string, error) {
	ipAddr := net.ParseIP(ip)
	if ipAddr == nil {
		return "", fmt.Errorf("invalid ip address")
	}
	return ipAddr.String(), nil
}

func parseMask(mask string) (int32, error) {
	ipMask := net.ParseIP(mask)
	if ipMask == nil {
		return 0, fmt.Errorf("invalid mask")
	}
	parsedMask := net.IPMask(ipMask.To4())
	prefixLen, _ := parsedMask.Size()
	return int32(prefixLen), nil
}
