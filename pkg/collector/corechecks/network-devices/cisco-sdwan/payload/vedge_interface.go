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

var vEdgeOperStatusMap = map[string]devicemetadata.IfOperStatus{
	"Up":   devicemetadata.OperStatusUp,
	"Down": devicemetadata.OperStatusDown,
}

var vEdgeAdminStatusMap = map[string]devicemetadata.IfAdminStatus{
	"Up":   devicemetadata.AdminStatusUp,
	"Down": devicemetadata.AdminStatusDown,
}

// VEdgeInterface is an implementation of CiscoInterface for vEdge devices
type VEdgeInterface struct {
	client.InterfaceState
}

// ID returns an unique interface ID
func (itf *VEdgeInterface) ID() string {
	// VmanageSystemIP is the device's System IP from vManage
	return fmt.Sprintf("%s:%s", itf.VmanageSystemIP, itf.Ifname)
}

// Index returns the interface index
func (itf *VEdgeInterface) Index() (int, error) {
	return int(itf.Ifindex), nil
}

// GetSpeedMbps returns the interface speed
func (itf *VEdgeInterface) GetSpeedMbps() int {
	speed, err := strconv.Atoi(itf.SpeedMbps)
	if err != nil {
		log.Warnf("Unable to parse vEdge interface %s speed %s", itf.Ifname, itf.SpeedMbps)
	}
	return speed
}

// OperStatus returns the interface oper status
func (itf *VEdgeInterface) OperStatus() devicemetadata.IfOperStatus {
	return convertOperStatus(vEdgeOperStatusMap, itf.IfOperStatus)
}

// AdminStatus returns the interface admin
func (itf *VEdgeInterface) AdminStatus() devicemetadata.IfAdminStatus {
	return convertAdminStatus(vEdgeAdminStatusMap, itf.IfAdminStatus)
}

// Metadata returns the interface metadata
func (itf *VEdgeInterface) Metadata(namespace string) (devicemetadata.InterfaceMetadata, error) {
	return devicemetadata.InterfaceMetadata{
		DeviceID:    fmt.Sprintf("%s:%s", namespace, itf.VmanageSystemIP), // VmanageSystemIP is the device's System IP from vManage
		IDTags:      []string{fmt.Sprintf("interface:%s", itf.Ifname)},
		Index:       int32(itf.Ifindex),
		Name:        itf.Ifname,
		Description: itf.Desc,
		MacAddress:  itf.Hwaddr,
		OperStatus:  convertOperStatus(vEdgeOperStatusMap, itf.IfOperStatus),
		AdminStatus: convertAdminStatus(vEdgeAdminStatusMap, itf.IfAdminStatus),
	}, nil
}

// IPV4AddressMetadata returns the metadata for this interface's IPV4 addresses
func (itf *VEdgeInterface) IPV4AddressMetadata(namespace string) (*devicemetadata.IPAddressMetadata, error) {
	return itf.buildIPMetadata(namespace, itf.IPAddress)
}

// IPV6AddressMetadata returns the metadata for this interface's IPV6 addresses
func (itf *VEdgeInterface) IPV6AddressMetadata(namespace string) (*devicemetadata.IPAddressMetadata, error) {
	return itf.buildIPMetadata(namespace, itf.Ipv6Address)
}

func (itf *VEdgeInterface) buildIPMetadata(namespace, ipAddress string) (*devicemetadata.IPAddressMetadata, error) {
	if isEmptyVEdgeIP(ipAddress) {
		return nil, nil
	}

	ip, prefixLen, err := parseVEdgeIP(ipAddress)
	if err != nil {
		return nil, err
	}

	return &devicemetadata.IPAddressMetadata{
		InterfaceID: fmt.Sprintf("%s:%s:%d", namespace, itf.VmanageSystemIP, int(itf.Ifindex)), // VmanageSystemIP is the device's System IP from vManage
		IPAddress:   ip,
		Prefixlen:   prefixLen,
	}, nil
}

func parseVEdgeIP(ip string) (string, int32, error) {
	ipaddr, ipv4Net, err := net.ParseCIDR(ip)
	if err != nil {
		return "", 0, err
	}
	prefixLen, _ := ipv4Net.Mask.Size()

	return ipaddr.String(), int32(prefixLen), nil
}

func isEmptyVEdgeIP(ip string) bool {
	return ip == "" || ip == "-"
}
