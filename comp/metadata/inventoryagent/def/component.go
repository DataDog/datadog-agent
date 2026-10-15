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
	// SetBatch applies a complete field set under one lock, so no reader
	// observes a partial batch. It does not change readiness: a closed payload
	// records the update but submits nothing. A nil value clears the cached
	// field.
	SetBatch(fields map[string]interface{})
	// SetReady opens or closes payload generation and every submission path.
	// Closing waits for in-flight generation and submission to finish. Callers
	// must serialize publication (close, update all fields, open, submit); an
	// embedding driver component owns that protocol, not the embedding binary's
	// wiring code.
	SetReady(ready bool)
	// Get returns a copy of the agent metadata. Useful to be incorporated in the status page.
	Get() map[string]interface{}
	// Submit synchronously builds a payload and enqueues it for submission now,
	// bypassing the metadata runner's first-run delay and interval gating.
	// Nothing is submitted while the payload is closed or disabled.
	Submit()
}

// Capabilities is an optional fx dependency letting an embedding binary adapt the
// inventoryagent component to an environment that diverges from the standard
// full-agent one. Present Capabilities also construct the payload closed until
// the first SetBatch opens it. Each field is named for its divergence, so the
// zero value is full-agent behavior apart from that closure.
type Capabilities struct {
	// SkipFullAgentMetadataRefresh skips all per-payload refreshMetadata collectors:
	// core, security, process, trace, system-probe, Fleet, and application monitoring.
	// Construction-time metadata, values supplied through Set and SetBatch, optional
	// configuration payloads, and submission scheduling are unaffected.
	SkipFullAgentMetadataRefresh bool
	// PayloadUUID overrides the payload's uuid, resolved per payload so an environment
	// that only learns its identity after construction still reports it. Nil uses the
	// cached host machine GUID. Called under the component lock: it must not call back in.
	PayloadUUID func() string
}

// NewServerlessCapabilities builds the Capabilities for serverless-init,
// reporting one uuid for the process lifetime since serverless containers do
// not share a host GUID.
func NewServerlessCapabilities(payloadUUID func() string) *Capabilities {
	return &Capabilities{
		SkipFullAgentMetadataRefresh: true,
		PayloadUUID:                  payloadUUID,
	}
}
