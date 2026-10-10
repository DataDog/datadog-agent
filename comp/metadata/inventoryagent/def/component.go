// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventoryagent implements a component to generate the 'datadog_agent' metadata payload for inventory.
package inventoryagent

// team: fleet-automation

// Component is the component type.
type Component interface {
	// Set updates a metadata value in the payload. The given value will be stored in the cache without being copied. It is
	// up to the caller to make sure the given value will not be modified later.
	Set(name string, value interface{})
	// Get returns a copy of the agent metadata. Useful to be incorporated in the status page.
	Get() map[string]interface{}
	// SetReady gates scheduled, explicit, and diagnostic payload generation without
	// enabling disabled inventory. SetReady(false) waits for in-flight generation.
	// Callers must serialize publication (close, update all fields and the UUID, open)
	// and must not hold metadata or UUID locks when calling SetReady or Submit.
	SetReady(ready bool)
	// Submit synchronously builds a payload and enqueues it for submission now,
	// bypassing the metadata runner's first-run delay and interval gating. Nothing
	// is submitted while not ready.
	Submit()
}

// Capabilities is an optional fx dependency letting an embedding binary adapt the
// inventoryagent component to an environment that diverges from the standard
// full-agent one. Each field is named for its divergence, so the zero value is
// full-agent behavior.
type Capabilities struct {
	// DeferUntilReady blocks payload generation from construction until the owner
	// calls SetReady(true); the provider is still registered when enabled.
	DeferUntilReady bool
	// SkipFullAgentMetadataRefresh skips all per-payload refreshMetadata collectors:
	// core, security, process, trace, system-probe, Fleet, and application monitoring.
	// Construction-time metadata, values supplied through Set, optional configuration
	// payloads, and submission scheduling are unaffected.
	SkipFullAgentMetadataRefresh bool
	// PayloadUUID overrides the payload's uuid, resolved per payload so an environment
	// that only learns its identity after construction still reports it. Nil uses the
	// cached host machine GUID. Called under the component lock: it must not call back in.
	PayloadUUID func() string
}

// NewServerlessCapabilities builds the Capabilities for serverless-init.
func NewServerlessCapabilities(payloadUUID func() string) *Capabilities {
	return &Capabilities{
		DeferUntilReady:              true,
		SkipFullAgentMetadataRefresh: true,
		PayloadUUID:                  payloadUUID,
	}
}
