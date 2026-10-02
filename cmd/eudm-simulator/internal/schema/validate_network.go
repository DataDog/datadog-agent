// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package schema

import (
	"fmt"
	"regexp"
	"strings"
)

// knownNDMMetrics is the set of metric names valid in network_metrics blocks.
// It mirrors the metric names emitted by ndmsim's Aruba access-point/wireless
// controller examples (ndmsim/internal/examples/snmp/aruba/*.go) and the
// generic interface metrics in ndmsim/internal/examples/helpers.go.
var knownNDMMetrics = map[string]bool{
	"snmp.cpu.usage":                      true,
	"snmp.memory.usage":                   true,
	"snmp.memory.used":                    true,
	"snmp.memory.total":                   true,
	"snmp.sysUpTimeInstance":              true,
	"snmp.device.reachable":               true,
	"snmp.apChannelBwRate":                true,
	"snmp.apChannelNoise":                 true,
	"snmp.apChannelFrameRetryRate":        true,
	"snmp.apChannelFrameReceiveErrorRate": true,
	"snmp.apBSSTxBytes":                   true,
	"snmp.apBSSRxBytes":                   true,
	"snmp.apBSSBwRate":                    true,
	"snmp.bsnApIfNoOfUsers":               true,
	"snmp.ifAdminStatus":                  true,
	"snmp.ifOperStatus":                   true,
	"snmp.ifHCInOctets":                   true,
	"snmp.ifHCOutOctets":                  true,
	"snmp.ifInErrors":                     true,
	"snmp.ifOutErrors":                    true,
	"snmp.ifInDiscards":                   true,
	"snmp.ifOutDiscards":                  true,
	"snmp.ifHighSpeed":                    true,
	"snmp.ifBandwidthInUsage.rate":        true,
	"snmp.ifBandwidthOutUsage.rate":       true,
}

// ndmMetricBounds defines [min, max] expected ranges for NDM metrics where an
// out-of-range value is clearly wrong: percentages/utilization rates, dBm
// noise readings, the boolean-shaped reachable flag, and the coded interface
// status enums. Mirrors metricBounds in validate.go for client metrics.
var ndmMetricBounds = map[string][2]float64{
	"snmp.cpu.usage":                      {0, 100},
	"snmp.memory.usage":                   {0, 100},
	"snmp.apChannelBwRate":                {0, 100},
	"snmp.apChannelFrameRetryRate":        {0, 100},
	"snmp.apChannelFrameReceiveErrorRate": {0, 100},
	"snmp.apBSSBwRate":                    {0, 100},
	"snmp.ifBandwidthInUsage.rate":        {0, 100},
	"snmp.ifBandwidthOutUsage.rate":       {0, 100},
	"snmp.apChannelNoise":                 {-120, 0},
	"snmp.device.reachable":               {0, 1},
	"snmp.ifAdminStatus":                  {1, 3},
	"snmp.ifOperStatus":                   {1, 7},
}

// macAddressPattern matches colon-separated 6-octet MAC/BSSID strings.
var macAddressPattern = regexp.MustCompile(`^[0-9a-fA-F]{2}(:[0-9a-fA-F]{2}){5}$`)

// validBands is the set of accepted NetworkInterfaceDef.Band values, required
// on radio interfaces to populate the NDM intake's wireless_interfaces record.
var validBands = map[string]bool{"2.4GHz": true, "5GHz": true, "6GHz": true}

// validateNetworkDevices checks the network_devices block: access point and
// interface declarations, fleet group associations, and network_metrics
// references. Errors are appended via add, matching the rest of Validate.
func validateNetworkDevices(s *Scenario, add func(format string, args ...any)) {
	namespace := "validation" // Replay supplies the opaque run namespace.

	apNames := map[string]bool{}
	apIPs := map[string]bool{}
	bssidOwner := map[string]string{}

	for i, ap := range s.NetworkDevices.AccessPoints {
		pfx := fmt.Sprintf("network_devices.access_points[%d]", i)
		if ap.Name != "" {
			pfx = fmt.Sprintf("network_devices.access_points[%d] (%q)", i, ap.Name)
		}

		if strings.TrimSpace(ap.Name) == "" {
			add("%s.name: required", pfx)
		} else if apNames[ap.Name] {
			add("%s.name: duplicate access point name %q", pfx, ap.Name)
		} else {
			apNames[ap.Name] = true
		}

		if strings.TrimSpace(ap.IPAddress) == "" {
			add("%s.ip_address: required", pfx)
		} else if apIPs[ap.IPAddress] {
			add("%s.ip_address: %q is already used by another access point — IPs must be unique", pfx, ap.IPAddress)
		} else {
			apIPs[ap.IPAddress] = true
		}

		if len(ap.Interfaces) == 0 {
			add("%s.interfaces: must define at least one interface", pfx)
		}

		ifaceNames := map[string]bool{}
		ifaceIndexes := map[int32]bool{}
		for j, iface := range ap.Interfaces {
			ipfx := fmt.Sprintf("%s.interfaces[%d]", pfx, j)
			if iface.Name != "" {
				ipfx = fmt.Sprintf("%s.interfaces[%d] (%q)", pfx, j, iface.Name)
			}

			if strings.TrimSpace(iface.Name) == "" {
				add("%s.name: required", ipfx)
			} else if ifaceNames[iface.Name] {
				add("%s.name: duplicate interface name %q on access point %q", ipfx, iface.Name, ap.Name)
			} else {
				ifaceNames[iface.Name] = true
			}

			if iface.Index < 1 {
				add("%s.index: required and must be >= 1 (ifIndex is 1-based; index 0 collides with the "+
					"AP's management-IP interface_id \"<device_id>:0\"), got %d", ipfx, iface.Index)
			} else if ifaceIndexes[iface.Index] {
				add("%s.index: duplicate index %d on access point %q", ipfx, iface.Index, ap.Name)
			} else {
				ifaceIndexes[iface.Index] = true
			}

			switch iface.Kind {
			case "ethernet", "radio":
			default:
				add("%s.kind: %q is not valid — must be one of: ethernet, radio", ipfx, iface.Kind)
			}

			if iface.MACAddress != "" && !macAddressPattern.MatchString(iface.MACAddress) {
				add("%s.mac_address: %q is not a valid MAC address (expected aa:bb:cc:dd:ee:ff)", ipfx, iface.MACAddress)
			}

			if iface.AdminStatus != 0 && (iface.AdminStatus < 1 || iface.AdminStatus > 3) {
				add("%s.admin_status: must be 1-3, got %d", ipfx, iface.AdminStatus)
			}
			if iface.OperStatus != 0 && (iface.OperStatus < 1 || iface.OperStatus > 7) {
				add("%s.oper_status: must be 1-7, got %d", ipfx, iface.OperStatus)
			}

			switch iface.Kind {
			case "radio":
				if iface.BSSID != "" && !macAddressPattern.MatchString(iface.BSSID) {
					add("%s.bssid: %q is not a valid BSSID (expected aa:bb:cc:dd:ee:ff)", ipfx, iface.BSSID)
					continue
				}
				effective := iface.EffectiveBSSID(namespace, ap.Name)
				if owner, ok := bssidOwner[effective]; ok {
					add("%s.bssid: effective BSSID %q collides with %s", ipfx, effective, owner)
				} else {
					bssidOwner[effective] = ipfx
				}
				if !validBands[iface.Band] {
					add("%s.band: required on radio interfaces — must be one of: 2.4GHz, 5GHz, 6GHz (got %q)", ipfx, iface.Band)
				}
			default:
				if iface.BSSID != "" {
					add("%s.bssid: only valid on radio interfaces", ipfx)
				}
				if iface.SSID != "" {
					add("%s.ssid: only valid on radio interfaces", ipfx)
				}
				if iface.Band != "" {
					add("%s.band: only valid on radio interfaces", ipfx)
				}
			}
		}
	}

	// ── fleet association checks ────────────────────────────────────────────
	for i, g := range s.Fleet {
		pfx := fmt.Sprintf("fleet[%d]", i)
		if g.Group != "" {
			pfx = fmt.Sprintf("fleet[%d] (%q)", i, g.Group)
		}
		if g.BSSID != "" && !macAddressPattern.MatchString(g.BSSID) {
			add("%s.bssid: expected a six-octet MAC address", pfx)
		}

		if g.BSSID != "" && g.AccessPoint != "" {
			add("%s: bssid and access_point are mutually exclusive — set only one", pfx)
			continue
		}

		if g.AccessPoint == "" {
			if g.Radio != "" {
				add("%s.radio: only valid together with access_point", pfx)
			}
			continue
		}

		ap := s.AccessPointByName(g.AccessPoint)
		if ap == nil {
			add("%s.access_point: %q does not match any declared access point", pfx, g.AccessPoint)
			continue
		}

		if g.Radio != "" {
			iface := ap.InterfaceByName(g.Radio)
			if iface == nil {
				add("%s.radio: %q is not an interface on access point %q", pfx, g.Radio, ap.Name)
			} else if iface.Kind != "radio" {
				add("%s.radio: %q is not a radio interface on access point %q", pfx, g.Radio, ap.Name)
			}
		} else if ap.FirstRadio() == nil {
			add("%s.access_point: %q has no radio interface to default to — set radio explicitly", pfx, ap.Name)
		}
	}

	// ── phase network_metrics checks ────────────────────────────────────────
	for i, ph := range s.Phases {
		pfx := fmt.Sprintf("phases[%d]", i)
		if ph.Name != "" {
			pfx = fmt.Sprintf("phases[%d] (%q)", i, ph.Name)
		}

		for apName, apm := range ph.NetworkMetrics {
			mpfx := fmt.Sprintf("%s.network_metrics[%s]", pfx, apName)
			ap := s.AccessPointByName(apName)
			if ap == nil {
				add("%s: %q does not match any declared access point", mpfx, apName)
				continue
			}

			for mname, pat := range apm.Device {
				validateNDMMetricPattern(mname, pat, fmt.Sprintf("%s.device.%s", mpfx, mname), add)
			}

			for ifaceName, metrics := range apm.Interfaces {
				ifpfx := fmt.Sprintf("%s.interfaces[%s]", mpfx, ifaceName)
				iface := ap.InterfaceByName(ifaceName)
				if iface == nil {
					add("%s: %q is not an interface on access point %q", ifpfx, ifaceName, apName)
					continue
				}
				for mname, pat := range metrics {
					validateNDMMetricPattern(mname, pat, fmt.Sprintf("%s.%s", ifpfx, mname), add)
				}
			}
		}
	}
}

// validateNDMMetricPattern checks an network_metrics entry: the metric name
// must be in the known NDM allowlist, the pattern must be structurally valid,
// and — for metrics with a known natural range — every value in the pattern
// must fall within it (mirrors client metric validation in validate.go).
func validateNDMMetricPattern(name string, pat Pattern, field string, add func(format string, args ...any)) {
	if !knownNDMMetrics[name] {
		add("%s: unknown NDM metric name — known metrics: %s", field, joinedSortedKeys(knownNDMMetrics))
		return
	}
	if isZeroPattern(pat) {
		add("%s: pattern is empty — set one of: steady, ramp, spike, step", field)
		return
	}
	if err := validatePattern(pat, field); err != nil {
		add("%s", err)
	}
	if bounds, ok := ndmMetricBounds[name]; ok {
		for _, msg := range patternBoundsErrors(pat, field, bounds[0], bounds[1]) {
			add("%s", msg)
		}
	}
}
