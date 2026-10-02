// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package inventoryagentimpl

import (
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

var _ serializer.InventoryCapture = (*Payload)(nil)

// CaptureInventoryStream identifies only this inventory contract.
func (*Payload) CaptureInventoryStream() telemetrycapture.Stream {
	return telemetrycapture.AgentInventory
}

// captureMetadata borrows only explicitly named scalar fields. It cannot copy
// arbitrary configuration strings, credentials, or remote-config identifiers.
func (p *Payload) captureMetadata() telemetrycapture.AgentInventoryMetadata {
	text := func(key string) string { v, _ := p.Metadata[key].(string); return v }
	flag := func(key string) bool { v, _ := p.Metadata[key].(bool); return v }
	startup, _ := p.Metadata["agent_startup_time_ms"].(int64)
	return telemetrycapture.AgentInventoryMetadata{
		AgentVersion: text("agent_version"), PackageVersion: text("package_version"),
		Flavor: text("flavor"), InfrastructureMode: text("infrastructure_mode"), AgentStartupTimeMS: startup,
		FeatureProcessEnabled:            flag("feature_process_enabled"),
		FeatureProcessesContainerEnabled: flag("feature_processes_container_enabled"),
		FeatureNetworksEnabled:           flag("feature_networks_enabled"),
		FeatureNetworksHTTPEnabled:       flag("feature_networks_http_enabled"),
		FeatureNetworksHTTPSEnabled:      flag("feature_networks_https_enabled"),
	}
}

// CaptureInventorySize measures the safe projection before owned copying.
func (p *Payload) CaptureInventorySize() int64 {
	a := p.captureMetadata()
	return 256 + telemetrycapture.InventorySize(&telemetrycapture.Inventory{Hostname: p.Hostname, UUID: p.UUID, Agent: &a})
}

// CopyCaptureInventory returns an independent credential-free projection.
func (p *Payload) CopyCaptureInventory() *telemetrycapture.Inventory {
	a := p.captureMetadata()
	return telemetrycapture.CloneInventory(&telemetrycapture.Inventory{Hostname: p.Hostname, UUID: p.UUID, Timestamp: p.Timestamp, Agent: &a})
}
