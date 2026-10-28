// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || darwin

package networkv2

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type linkState int

const (
	linkStateUnknown linkState = iota
	linkStateUp
	linkStateDown
)

// ifaceInfo describes one network interface. Each platform fills the fields it can get, and
// leaves the others at their zero value.
type ifaceInfo struct {
	name                string
	description         string
	ifaceType           string // ethernet, wifi, cellular, loopback, tunnel, virtual or other
	macAddress          string
	permanentMacAddress string
	mtu                 uint32
	link                linkState
	speedMbps           float64
	duplex              string // full or half
	vlanID              uint32
	builtin             *bool
	vendor              string
	model               string
	driver              string
	driverProvider      string
	driverVersion       string
}

// tags returns the tags of the interface gauges.
func (i ifaceInfo) tags() []string {
	tags := []string{"iface:" + i.name, "iface_type:" + i.ifaceType}
	if i.duplex != "" {
		tags = append(tags, "duplex:"+i.duplex)
	}
	if i.vlanID != 0 {
		tags = append(tags, "vlan_id:"+strconv.FormatUint(uint64(i.vlanID), 10))
	}
	return tags
}

// submitIfaceMetrics submits the MTU, link state and link speed gauges of an interface.
func submitIfaceMetrics(sender sender.Sender, iface ifaceInfo) {
	tags := iface.tags()
	if iface.mtu != 0 {
		sender.Gauge("system.net.iface.mtu", float64(iface.mtu), "", tags)
	}
	switch iface.link {
	case linkStateUp:
		sender.Gauge("system.net.iface.up", 1, "", tags)
		// Only Ethernet negotiates its speed. Windows reports made-up speeds for virtual interfaces,
		// and the speed of a Wi-Fi interface on macOS is a driver estimate rather than the link rate
		// of system.wlan.txrate.
		if iface.speedMbps > 0 && iface.ifaceType == "ethernet" {
			sender.Gauge("system.net.iface.speed", iface.speedMbps, "", tags)
		}
	case linkStateDown:
		sender.Gauge("system.net.iface.up", 0, "", tags)
	}
}

// nicMetadata describes one interface in the "interfaces" check metadata. It leaves out the
// link state and speed, which change often, because every change sends a new inventory payload.
type nicMetadata struct {
	Name                string `json:"name"`
	Description         string `json:"description,omitempty"`
	Type                string `json:"type"`
	MacAddress          string `json:"macaddress,omitempty"`
	PermanentMacAddress string `json:"permanent_macaddress,omitempty"`
	MTU                 uint32 `json:"mtu,omitempty"`
	Duplex              string `json:"duplex,omitempty"`
	VlanID              uint32 `json:"vlan_id,omitempty"`
	Builtin             *bool  `json:"builtin,omitempty"`
	Vendor              string `json:"vendor,omitempty"`
	Model               string `json:"model,omitempty"`
	Driver              string `json:"driver,omitempty"`
	DriverProvider      string `json:"driver_provider,omitempty"`
	DriverVersion       string `json:"driver_version,omitempty"`
}

// newNicMetadata returns the metadata of an interface, with its permanent MAC address only when it
// differs from the current one.
func newNicMetadata(iface ifaceInfo) nicMetadata {
	nic := nicMetadata{
		Name:           iface.name,
		Description:    iface.description,
		Type:           iface.ifaceType,
		MacAddress:     iface.macAddress,
		MTU:            iface.mtu,
		Duplex:         iface.duplex,
		VlanID:         iface.vlanID,
		Builtin:        iface.builtin,
		Vendor:         iface.vendor,
		Model:          iface.model,
		Driver:         iface.driver,
		DriverProvider: iface.driverProvider,
		DriverVersion:  iface.driverVersion,
	}
	if iface.permanentMacAddress != iface.macAddress {
		nic.PermanentMacAddress = iface.permanentMacAddress
	}
	return nic
}

// setInterfacesMetadata sets the "interfaces" check metadata to the interfaces, sorted by name.
func (c *NetworkCheck) setInterfacesMetadata(ifaces []ifaceInfo) {
	inv, err := check.GetInventoryChecksContext()
	if err != nil {
		return
	}
	nics := make([]nicMetadata, 0, len(ifaces))
	for _, iface := range ifaces {
		nics = append(nics, newNicMetadata(iface))
	}
	slices.SortFunc(nics, func(a, b nicMetadata) int { return strings.Compare(a.Name, b.Name) })
	payload, err := json.Marshal(nics)
	if err != nil {
		log.Debugf("Unable to marshal network interfaces metadata: %s", err)
		return
	}
	// Check metadata values are strings, as for Python checks
	inv.Set(string(c.ID()), "interfaces", string(payload))
}
