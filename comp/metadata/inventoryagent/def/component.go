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
	// enabling disabled inventory. Closing waits for in-flight generation/enqueue.
	// Callers must serialize publication (close, update all fields/UUID, open), and
	// must not hold metadata or UUID locks when calling SetReady or Submit.
	SetReady(ready bool)
	// Submit synchronously builds a payload and enqueues it for submission now,
	// bypassing the metadata runner's first-run delay and interval gating. An
	// embedder with no host-metadata pipeline has no host-creation race to order
	// around and may exit before the runner goroutine fires, so it enqueues the
	// first payload directly. Nothing is submitted while not ready.
	Submit()
}

// Capabilities lets an embedding binary adapt the inventoryagent component for
// an environment that diverges from the standard full-agent one (currently:
// serverless-init). It is an optional fx dependency, and each field is named
// for its divergence so the zero value (no Capabilities supplied) is exactly
// full-agent behavior.
type Capabilities struct {
	// DeferUntilReady blocks payload generation from construction until the owner
	// calls SetReady(true) after initializing all metadata and identity. A deferred
	// provider is still registered when enabled; false retains default readiness.
	DeferUntilReady bool
	// SkipFullAgentMetadataRefresh skips all per-payload refreshMetadata collectors:
	// core, security, process, trace, system-probe, Fleet, and application monitoring.
	// This includes local configuration and file reads, not just cross-process IPC.
	// Construction-time metadata, values supplied through Set, optional configuration
	// payloads, and submission scheduling remain enabled. This is an internal Fx
	// capability, not a YAML or environment setting; false retains full-agent refresh.
	SkipFullAgentMetadataRefresh bool
	// PayloadUUID overrides the payload's uuid. Nil means use the cached host
	// machine GUID (uuid.GetUUID()), which is meaningless across the ephemeral,
	// per-process containers of a hostless environment. Resolved per payload, so an
	// environment that only learns its identity after construction still reports it.
	// Called while the component holds its lock: it must not call back in.
	PayloadUUID func() string
}

// NewServerlessCapabilities builds the Capabilities for serverless-init, a
// hostless single-process environment with no sibling agent processes. The owner
// must call SetReady(true) after publishing its metadata and identity, then Submit
// to enqueue immediately without waiting for the scheduled collection delay.
func NewServerlessCapabilities(payloadUUID func() string) *Capabilities {
	return &Capabilities{
		DeferUntilReady:              true,
		SkipFullAgentMetadataRefresh: true,
		PayloadUUID:                  payloadUUID,
	}
}
