// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package accesspoint generates Agent-owned NDM evidence for scenario radios.
package accesspoint

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/identity"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

const (
	// MetricsCadence matches the Agent metric flush interval.
	MetricsCadence = 15 * time.Second
	// MetadataCadence refreshes the NDM resource inventory at its normal cadence.
	MetadataCadence = 5 * time.Minute
)

type accessPoint struct {
	definition schema.AccessPointDef
	device     metadata.DeviceMetadata
	interfaces []metadata.InterfaceMetadata
	wireless   []metadata.WirelessInterfaceMetadata
	address    metadata.IPAddressMetadata
}

// Model is immutable after construction. Metrics are evaluated from phase
// declarations, so delivery order never changes carry-forward or variation.
type Model struct {
	scenario     *schema.Scenario
	runID        string
	namespace    string
	seed         uint64
	accessPoints []accessPoint
}

// New builds opaque NDM identities while retaining declared hardware facts.
func New(s *schema.Scenario, runID string, seed uint64) (*Model, error) {
	if s == nil {
		return nil, fmt.Errorf("access points require a scenario")
	}
	if err := s.Validate(); err != nil {
		return nil, err
	}
	if decoded, err := hex.DecodeString(runID); err != nil || len(decoded) != 16 {
		return nil, fmt.Errorf("access points require an opaque run ID")
	}
	if len(s.NetworkDevices.AccessPoints) > 65534 {
		return nil, fmt.Errorf("too many access points for the run address space")
	}
	m := &Model{scenario: s, runID: runID, namespace: identity.Namespace(runID), seed: seed}
	addressBlock := sha256.Sum256([]byte(runID))
	for ordinal, def := range s.NetworkDevices.AccessPoints {
		ip := fmt.Sprintf("10.%d.%d.%d", addressBlock[0], (ordinal+1)>>8, (ordinal+1)&255)
		deviceID := metadata.DeviceID(m.namespace, ip)
		name := "ap-" + runID + "-" + opaque(def.Name)
		d := metadata.DeviceMetadata{ID: deviceID, IDTags: metadata.DeviceIDTags(m.namespace, ip), IPAddress: ip, Name: name, OsHostname: name, SerialNumber: "serial-" + opaque(runID, def.SerialNumber, def.Name), Status: metadata.DeviceStatusReachable, PingStatus: metadata.DeviceStatusReachable, Profile: def.Profile, Vendor: def.Vendor, Model: def.Model, OsName: def.OSName, OsVersion: def.OSVersion, Version: def.Version, SysObjectID: def.SysObjectID, Integration: s.EffectiveIntegration(), DeviceType: def.DeviceType}
		if d.DeviceType == "" {
			d.DeviceType = "access_point"
		}
		if def.Location != "" {
			d.Location = "location-" + opaque(runID, def.Location)
		}
		d.Tags = []string{"device_namespace:" + m.namespace, "device_id:" + deviceID, "device_ip:" + ip, "snmp_device:" + ip, "device_hostname:" + name, "eudm_run_id:" + runID, "device_type:" + d.DeviceType}
		for key, value := range map[string]string{"device_vendor": def.Vendor, "snmp_profile": def.Profile} {
			if value != "" {
				d.Tags = append(d.Tags, key+":"+value)
			}
		}
		// Declaration names and identity tags must not bypass the identity map.
		for _, tag := range def.Tags {
			key, _, _ := strings.Cut(tag, ":")
			if slices.Contains([]string{"device_namespace", "device_id", "device_ip", "snmp_device", "device_hostname", "serial_number", "mac_address", "bssid", "ssid", "eudm_run_id", "dd.internal.resource"}, key) {
				continue
			}
			hidden := strings.Contains(strings.ToLower(tag), strings.ToLower(s.Meta.Name))
			for _, ap := range s.NetworkDevices.AccessPoints {
				hidden = hidden || strings.Contains(strings.ToLower(tag), strings.ToLower(ap.Name))
			}
			if !hidden {
				d.Tags = append(d.Tags, tag)
			}
		}
		slices.Sort(d.Tags)
		d.Tags = slices.Compact(d.Tags)
		ap := accessPoint{definition: def, device: d}
		for _, iface := range def.Interfaces {
			ifaceName := "if-" + opaque(runID, def.Name, iface.Name)
			mac := identity.RadioMAC(runID, def.Name, iface.Name, iface.MACAddress)
			typeID := int32(6)
			if iface.Kind == "radio" {
				typeID = 71
				mac = identity.RadioMAC(runID, def.Name, iface.Name, iface.BSSID)
				ap.wireless = append(ap.wireless, metadata.WirelessInterfaceMetadata{Namespace: m.namespace, DeviceByIntegrationID: deviceID, InterfaceByIntegrationID: metadata.InterfaceID(deviceID, iface.Index), BSSID: mac, SSID: identity.SSID(runID, iface.EffectiveSSID()), Band: iface.Band, AdminStatus: metadata.IfAdminStatus(iface.EffectiveAdminStatus()), OperStatus: metadata.IfOperStatus(iface.EffectiveOperStatus()), Tags: []string{"eudm_run_id:" + runID}})
			}
			ap.interfaces = append(ap.interfaces, metadata.InterfaceMetadata{DeviceID: deviceID, IDTags: metadata.InterfaceIDTags(ifaceName, iface.Index), Index: iface.Index, Name: ifaceName, MacAddress: mac, AdminStatus: metadata.IfAdminStatus(iface.EffectiveAdminStatus()), OperStatus: metadata.IfOperStatus(iface.EffectiveOperStatus()), Type: typeID})
		}
		// The management address belongs to the first declared physical interface,
		// falling back to the first radio for an AP without an ethernet port.
		managementIndex := def.Interfaces[0].Index
		for _, iface := range def.Interfaces {
			if iface.Kind == "ethernet" {
				managementIndex = iface.Index
				break
			}
		}
		ap.address = metadata.IPAddressMetadata{InterfaceID: metadata.InterfaceID(deviceID, managementIndex), IPAddress: ip, Prefixlen: 24}
		m.accessPoints = append(m.accessPoints, ap)
	}
	return m, nil
}

// Wireless returns the exact BSSID/SSID pair written to the NDM radio resource.
// A cohort without an AP association uses its captured wireless baseline instead.
func (m *Model) Wireless(group schema.GroupDef) (*identity.Wireless, error) {
	if group.AccessPoint == "" {
		return nil, nil
	}
	for _, ap := range m.accessPoints {
		if ap.definition.Name != group.AccessPoint {
			continue
		}
		radio := ap.definition.FirstRadio()
		if group.Radio != "" {
			radio = ap.definition.InterfaceByName(group.Radio)
		}
		if radio == nil || radio.Kind != "radio" {
			return nil, fmt.Errorf("cohort requires a declared AP radio")
		}
		for _, wireless := range ap.wireless {
			if wireless.InterfaceByIntegrationID == metadata.InterfaceID(ap.device.ID, radio.Index) {
				return &identity.Wireless{BSSID: wireless.BSSID, SSID: wireless.SSID}, nil
			}
		}
	}
	return nil, fmt.Errorf("cohort access point is not in this model")
}

// Metadata returns owned Agent payloads, accounting for wireless resources in
// the same batch limit as device, interface, and address resources.
func (m *Model) Metadata(at time.Time, batchSize int) []metadata.NetworkDevicesMetadata {
	if len(m.accessPoints) == 0 {
		return nil
	}
	if batchSize <= 0 {
		batchSize = metadata.PayloadMetadataBatchSize
	}
	var devices []metadata.DeviceMetadata
	var interfaces []metadata.InterfaceMetadata
	var wireless []metadata.WirelessInterfaceMetadata
	var addresses []metadata.IPAddressMetadata
	for _, ap := range m.accessPoints {
		device := ap.device
		device.IDTags = slices.Clone(device.IDTags)
		device.Tags = slices.Clone(device.Tags)
		devices = append(devices, device)
		for _, source := range ap.interfaces {
			iface := source
			iface.IDTags = slices.Clone(iface.IDTags)
			interfaces = append(interfaces, iface)
		}
		for _, source := range ap.wireless {
			iface := source
			iface.Tags = slices.Clone(iface.Tags)
			wireless = append(wireless, iface)
		}
		addresses = append(addresses, ap.address)
	}
	return metadata.BatchPayloadsWithWirelessInterfaces(integrations.Integration(m.scenario.EffectiveIntegration()), m.namespace, "", at, batchSize, devices, interfaces, wireless, addresses, nil, nil, nil, nil)
}

func opaque(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		var size [8]byte
		binary.BigEndian.PutUint64(size[:], uint64(len(part)))
		h.Write(size[:])
		h.Write([]byte(part))
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}
