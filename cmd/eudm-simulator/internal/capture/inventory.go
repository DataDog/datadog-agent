// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package capture

import (
	"errors"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	tc "github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// Inventory sanitizes an explicitly projected inventory envelope. Timestamps
// become relative to the same origin as all other streams, preserving uptime.
// Neither the input nor its nested metadata is modified or retained.
func (s *Sanitizer) Inventory(in *tc.Inventory, origin time.Time) (*tc.Inventory, error) {
	if in == nil {
		return nil, errors.New("missing inventory projection")
	}
	kinds := 0
	if in.Agent != nil {
		kinds++
	}
	if in.Host != nil {
		kinds++
	}
	if in.SystemInfo != nil {
		kinds++
	}
	if in.Hostname == "" || in.UUID == "" || in.Timestamp < origin.UnixNano() || kinds != 1 {
		return nil, errors.New("inventory lacks a current observation or a unique metadata type")
	}
	out := &tc.Inventory{Hostname: "capture-host", UUID: s.uuid(in.UUID), Timestamp: in.Timestamp - origin.UnixNano()}
	if a := in.Agent; a != nil {
		if !agentVersionPattern.MatchString(a.AgentVersion) || a.Flavor != "agent" || a.InfrastructureMode != "end_user_device" || a.AgentStartupTimeMS <= 0 || a.AgentStartupTimeMS > in.Timestamp/int64(time.Millisecond) {
			return nil, errors.New("Agent inventory requires an observed EUDM Agent identity and startup time")
		}
		clean := *a
		clean.AgentStartupTimeMS -= origin.UnixMilli()
		if !agentVersionPattern.MatchString(clean.PackageVersion) {
			clean.PackageVersion = ""
		}
		out.Agent = &clean
	}
	if h := in.Host; h != nil {
		if !agentVersionPattern.MatchString(h.AgentVersion) || safeOSPlatform(h.OS) == "" || h.MemoryTotalKb == 0 || h.CPUFrequency < 0 || math.IsNaN(h.CPUFrequency) || math.IsInf(h.CPUFrequency, 0) {
			return nil, errors.New("host inventory lacks supported operating system or resource evidence")
		}
		// Reconstruct the allowlist rather than retaining opaque kernel text,
		// DMI identifiers, cloud relationships, or newly added producer fields.
		out.Host = &tc.HostInventoryMetadata{
			CPUCores: h.CPUCores, CPULogicalProcessors: h.CPULogicalProcessors,
			CPUModelID: safeVersion(h.CPUModelID), CPUFamily: safeVersion(h.CPUFamily), CPUStepping: safeVersion(h.CPUStepping),
			CPUFrequency: h.CPUFrequency, CPUCacheSize: h.CPUCacheSize,
			KernelName: safeOSPlatform(h.KernelName), KernelRelease: safeVersion(h.KernelRelease),
			OS: h.OS, OSVersion: safeOSVersion(h.OSVersion), CPUArchitecture: safeArchitecture(h.CPUArchitecture),
			MemoryTotalKb: h.MemoryTotalKb, MemorySwapTotalKb: h.MemorySwapTotalKb,
			IPAddress: s.ip(h.IPAddress), IPv6Address: s.ip(h.IPv6Address), MacAddress: s.mac(h.MacAddress),
			AgentVersion: h.AgentVersion,
		}
		if slices.Contains([]string{"Apple", "ARM", "GenuineIntel", "AuthenticAMD", "Intel", "AMD"}, h.CPUVendor) {
			out.Host.CPUVendor = h.CPUVendor
		} else {
			out.Host.CPUVendor = s.token("cpu_vendor", h.CPUVendor)
		}
		if appleCPUModelPattern.MatchString(strings.ReplaceAll(h.CPUModel, " ", "_")) {
			out.Host.CPUModel = h.CPUModel
		} else {
			out.Host.CPUModel = s.token("cpu_model", h.CPUModel)
		}
	}
	if h := in.SystemInfo; h != nil {
		modelName := func(value string) string {
			if appleHardwarePattern.MatchString(value) || slices.Contains([]string{"MacBook Pro", "MacBook Air", "MacBook", "iMac", "Mac mini", "Mac Pro", "Mac Studio"}, value) {
				return value
			}
			return s.token("device_model", value)
		}
		manufacturer := h.Manufacturer
		if !slices.Contains([]string{"Apple Inc.", "Apple", "Dell Inc.", "Dell", "LENOVO", "Lenovo", "HP", "Hewlett-Packard", "Microsoft Corporation", "ASUSTeK COMPUTER INC.", "Acer", "Samsung"}, manufacturer) {
			manufacturer = s.token("hardware_vendor", manufacturer)
		}
		chassis := h.ChassisType
		if !slices.Contains([]string{"", "Laptop", "Desktop", "Virtual Machine", "Other"}, chassis) {
			return nil, errors.New("unsupported observed chassis type")
		}
		out.SystemInfo = &tc.HostSystemInfoMetadata{Manufacturer: manufacturer, ModelNumber: modelName(h.ModelNumber), ModelName: modelName(h.ModelName), Identifier: modelName(h.Identifier), ChassisType: chassis, SerialNumber: s.token("serial_number", h.SerialNumber)}
	}
	return out, nil
}

var appleHardwarePattern = regexp.MustCompile(`^(?:Mac|MacBookPro|MacBookAir|MacBook|iMac|Macmini|MacPro|MacStudio)[0-9]{1,2},[0-9]{1,2}$`)
