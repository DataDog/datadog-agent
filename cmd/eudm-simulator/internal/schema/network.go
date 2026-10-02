// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package schema

import (
	"fmt"
	"hash/fnv"
	"strings"
)

// NetworkDevicesConfig declares the simulated network devices (access points)
// for a simulator scenario. When AccessPoints is empty, eudsim sends no NDM
// metadata and evaluates no AP metrics — the scenario behaves exactly as
// before this block existed.
type NetworkDevicesConfig struct {
	Integration  string           `yaml:"integration"` // optional; default "snmp"
	AccessPoints []AccessPointDef `yaml:"access_points"`
}

// AccessPointDef describes one simulated NDM access-point device. Fields
// mirror the NDM intake's device identity surface (see
// ndmsim/internal/device/device.go DeviceSpec).
type AccessPointDef struct {
	Name         string                `yaml:"name"`          // required; unique; used as sysName
	IPAddress    string                `yaml:"ip_address"`    // required; unique; primary IP
	Vendor       string                `yaml:"vendor"`        // e.g. "aruba"
	Model        string                `yaml:"model"`         // e.g. "Aruba AP-535"
	DeviceType   string                `yaml:"device_type"`   // e.g. "access_point"
	Profile      string                `yaml:"profile"`       // e.g. "aruba-access-point"
	SysObjectID  string                `yaml:"sys_object_id"` // e.g. "1.3.6.1.4.1.14823.1.1.57"
	SerialNumber string                `yaml:"serial_number"`
	OSName       string                `yaml:"os_name"`
	OSVersion    string                `yaml:"os_version"`
	Version      string                `yaml:"version"`
	Location     string                `yaml:"location"`
	Description  string                `yaml:"description"`
	Tags         []string              `yaml:"tags"`
	Interfaces   []NetworkInterfaceDef `yaml:"interfaces"` // required; at least one
}

// NetworkInterfaceDef describes one interface on an AccessPointDef. Kind is
// "ethernet" or "radio"; radio interfaces additionally carry an optional
// literal BSSID override (empty = derive one deterministically), a broadcast
// SSID, and a band — required so the NDM intake's wireless_interfaces record
// (distinct from the generic interfaces record) can be populated.
type NetworkInterfaceDef struct {
	Kind        string `yaml:"kind"` // required; "ethernet" | "radio"
	Index       int32  `yaml:"index"`
	Name        string `yaml:"name"`
	Alias       string `yaml:"alias"`
	Description string `yaml:"description"`
	MACAddress  string `yaml:"mac_address"`
	Speed       int64  `yaml:"speed"`
	AdminStatus int    `yaml:"admin_status"` // IANA ifAdminStatus (1=up,2=down,3=testing); default 1
	OperStatus  int    `yaml:"oper_status"`  // IANA ifOperStatus (1-7); default 1
	BSSID       string `yaml:"bssid"`        // radio only; literal override; empty = derive
	SSID        string `yaml:"ssid"`         // radio only; broadcast SSID; default "Corp-WiFi"
	Band        string `yaml:"band"`         // radio only; required; one of "2.4GHz", "5GHz", "6GHz"
}

// APPhaseMetrics is the per-AP metric block for one phase: device-level
// metrics plus a nested map of per-interface metrics. Nesting (rather than a
// flat "ap/iface" composite key) avoids parsing ambiguity on interface names
// that contain separators.
type APPhaseMetrics struct {
	Device     map[string]Pattern            `yaml:"device"`
	Interfaces map[string]map[string]Pattern `yaml:"interfaces"`
}

// InterfaceByName returns the named interface on this access point, or nil.
func (a *AccessPointDef) InterfaceByName(name string) *NetworkInterfaceDef {
	for i := range a.Interfaces {
		if a.Interfaces[i].Name == name {
			return &a.Interfaces[i]
		}
	}
	return nil
}

// FirstRadio returns the first radio-kind interface declared on this access
// point, or nil if it has none. Used as the default radio for a fleet group
// that sets access_point without an explicit radio.
func (a *AccessPointDef) FirstRadio() *NetworkInterfaceDef {
	for i := range a.Interfaces {
		if a.Interfaces[i].Kind == "radio" {
			return &a.Interfaces[i]
		}
	}
	return nil
}

// EffectiveBSSID returns this interface's BSSID: the literal value if set,
// otherwise one derived deterministically from namespace|apName|ifaceName via
// FNV-64a, with the locally-administered bit set and the multicast bit
// cleared on the first octet (mirrors device.SyntheticBSSID in
// internal/device/templates.go). Same namespace + AP + interface name always
// yields the same BSSID, on every run.
func (iface *NetworkInterfaceDef) EffectiveBSSID(namespace, apName string) string {
	if iface.BSSID != "" {
		return strings.ToLower(iface.BSSID)
	}
	h := fnv.New64a()
	h.Write([]byte(fmt.Sprintf("bssid:%s|%s|%s", namespace, apName, iface.Name)))
	v := h.Sum64()
	b0 := byte((v >> 40) & 0xff)
	b0 = (b0 | 0x02) &^ 0x01 // set locally-administered, clear multicast
	bssid := fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
		b0, (v>>32)&0xff, (v>>24)&0xff, (v>>16)&0xff, (v>>8)&0xff, v&0xff)
	return strings.ToLower(bssid)
}

// EffectiveSSID returns this radio's broadcast SSID, defaulting to "Corp-WiFi".
func (iface *NetworkInterfaceDef) EffectiveSSID() string {
	if iface.SSID != "" {
		return iface.SSID
	}
	return "Corp-WiFi"
}

// EffectiveAdminStatus returns AdminStatus, defaulting to 1 (up) when unset.
func (iface *NetworkInterfaceDef) EffectiveAdminStatus() int {
	if iface.AdminStatus == 0 {
		return 1
	}
	return iface.AdminStatus
}

// EffectiveOperStatus returns OperStatus, defaulting to 1 (up) when unset.
func (iface *NetworkInterfaceDef) EffectiveOperStatus() int {
	if iface.OperStatus == 0 {
		return 1
	}
	return iface.OperStatus
}
