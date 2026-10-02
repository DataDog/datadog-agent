// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package inventoryhostimpl

import (
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

var _ serializer.InventoryCapture = (*Payload)(nil)

// CaptureInventoryStream identifies only this inventory contract.
func (*Payload) CaptureInventoryStream() telemetrycapture.Stream {
	return telemetrycapture.HostInventory
}

func (p *Payload) captureMetadata() telemetrycapture.HostInventoryMetadata {
	h := p.Metadata
	return telemetrycapture.HostInventoryMetadata{
		KernelVersion:                h.KernelVersion,
		Interfaces:                   h.Interfaces,
		CloudProvider:                h.CloudProvider,
		CloudProviderSource:          h.CloudProviderSource,
		CloudProviderAccountID:       h.CloudProviderAccountID,
		CloudProviderHostID:          h.CloudProviderHostID,
		CanonicalCloudResourceID:     h.CanonicalCloudResourceID,
		InstanceType:                 h.InstanceType,
		HypervisorGuestUUID:          h.HypervisorGuestUUID,
		DmiProductUUID:               h.DmiProductUUID,
		DmiBoardAssetTag:             h.DmiBoardAssetTag,
		DmiBoardVendor:               h.DmiBoardVendor,
		LinuxPackageSigningEnabled:   h.LinuxPackageSigningEnabled,
		RPMGlobalRepoGPGCheckEnabled: h.RPMGlobalRepoGPGCheckEnabled,

		CPUCores: h.CPUCores, CPULogicalProcessors: h.CPULogicalProcessors, CPUVendor: h.CPUVendor,
		CPUModel: h.CPUModel, CPUModelID: h.CPUModelID, CPUFamily: h.CPUFamily, CPUStepping: h.CPUStepping,
		CPUFrequency: h.CPUFrequency, CPUCacheSize: h.CPUCacheSize,
		KernelName: h.KernelName, KernelRelease: h.KernelRelease, OS: h.OS, OSVersion: h.OsVersion, CPUArchitecture: h.CPUArchitecture,
		MemoryTotalKb: h.MemoryTotalKb, MemorySwapTotalKb: h.MemorySwapTotalKb,
		IPAddress: h.IPAddress, IPv6Address: h.IPv6Address, MacAddress: h.MacAddress, AgentVersion: h.AgentVersion,
	}
}

// CaptureInventorySize measures only the safe scalar projection.
func (p *Payload) CaptureInventorySize() int64 {
	if p.Metadata == nil {
		return 256
	}
	h := p.captureMetadata()
	return 256 + telemetrycapture.InventorySize(&telemetrycapture.Inventory{Hostname: p.Hostname, UUID: p.UUID, Host: &h})
}

// CopyCaptureInventory preserves native host telemetry; device identities
// are normalized by the coordinator rather than discarded here.
func (p *Payload) CopyCaptureInventory() *telemetrycapture.Inventory {
	if p.Metadata == nil {
		return nil
	}
	h := p.captureMetadata()
	return telemetrycapture.CloneInventory(&telemetrycapture.Inventory{Hostname: p.Hostname, UUID: p.UUID, Timestamp: p.Timestamp, Host: &h})
}
