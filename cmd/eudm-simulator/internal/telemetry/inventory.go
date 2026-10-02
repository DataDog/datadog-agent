// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"errors"
	"math"
	"net"
	"net/netip"
	"strings"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func validateInventory(stream schema.Stream, value *telemetrycapture.Inventory) error {
	if value.Hostname == "" || value.UUID == "" || value.Timestamp < 0 {
		return errors.New("inventory lacks host identity or relative collection time")
	}
	if stream == schema.AgentInventory {
		a := value.Agent
		if a == nil || value.Host != nil || value.SystemInfo != nil || a.AgentVersion == "" || a.Flavor == "" || a.InfrastructureMode != "end_user_device" || a.AgentStartupTimeMS > value.Timestamp/1e6 {
			return errors.New("Agent inventory requires observed end_user_device mode, version, flavor, and valid startup time")
		}
		return nil
	}
	if stream == schema.HostSystemInfo {
		h := value.SystemInfo
		if h == nil || value.Agent != nil || value.Host != nil || (h.Manufacturer == "" && h.ModelName == "" && h.ModelNumber == "" && h.Identifier == "") {
			return errors.New("system information lacks a unique observed hardware projection")
		}
		return nil
	}
	h := value.Host
	if h == nil || value.Agent != nil || value.SystemInfo != nil || h.AgentVersion == "" || InventoryPlatform(h.OS) == "" || h.CPUCores == 0 || h.CPULogicalProcessors < h.CPUCores || h.MemoryTotalKb == 0 || h.MemoryTotalKb > math.MaxUint64/1024 || h.CPUFrequency < 0 || math.IsNaN(h.CPUFrequency) || math.IsInf(h.CPUFrequency, 0) {
		return errors.New("host inventory lacks supported OS, version, or valid CPU and memory resources")
	}
	if h.KernelName != "" && InventoryPlatform(h.KernelName) != InventoryPlatform(h.OS) {
		return errors.New("host inventory kernel and OS disagree")
	}
	for _, address := range []struct {
		value string
		ipv6  bool
	}{{h.IPAddress, false}, {h.IPv6Address, true}} {
		if address.value != "" {
			ip, err := netip.ParseAddr(address.value)
			if err != nil || ip.Is6() != address.ipv6 {
				return errors.New("host inventory contains an invalid primary IP address")
			}
		}
	}
	if h.MacAddress != "" {
		if _, err := net.ParseMAC(h.MacAddress); err != nil {
			return errors.New("host inventory contains an invalid primary MAC address")
		}
	}
	return nil
}

// InventoryPlatform normalizes the native platform spelling without inventing
// platform evidence that the inventory producer did not observe.
func InventoryPlatform(value string) string {
	value = strings.ToLower(value)
	if strings.HasPrefix(value, "windows ") || strings.HasPrefix(value, "microsoft windows ") {
		return "windows"
	}
	switch value {
	case "darwin", "macos":
		return "macos"
	case "windows", "win32":
		return "windows"
	default:
		return ""
	}
}
