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
	// SetBatch applies a complete field set and the payload uuid atomically,
	// under one lock: no reader observes the previous uuid or a partial batch
	// once SetBatch returns. An empty uuid keeps the host machine GUID. A nil
	// value clears the cached field.
	SetBatch(fields map[string]interface{}, uuid string)
	// Get returns a copy of the agent metadata. Useful to be incorporated in the status page.
	Get() map[string]interface{}
	// Submit synchronously builds a payload and enqueues it for submission now,
	// bypassing the metadata runner's first-run delay and interval gating.
	// Nothing is submitted while the payload is disabled or fails validation.
	Submit()
}

// Capabilities is an optional fx dependency letting an embedding binary adapt the
// inventoryagent component to an environment that diverges from the standard
// full-agent one. Each field is named for its divergence, so the zero value is
// full-agent behavior.
type Capabilities struct {
	// Disabled withholds the payload entirely: no provider registration, no
	// submission, no construction-time collection. The embedding binary uses it
	// when its environment makes the payload uncollectable; the configuration
	// keys remain authoritative for their own side, so a disabled pipeline can
	// never be re-enabled this way.
	Disabled bool
	// SkipFullAgentMetadataRefresh skips all per-payload refreshMetadata collectors:
	// core, security, process, trace, system-probe, Fleet, and application monitoring.
	// Construction-time metadata, values supplied through Set and SetBatch, optional
	// configuration payloads, and submission scheduling are unaffected.
	SkipFullAgentMetadataRefresh bool
	// ValidatePayload is consulted before every submission, on the metadata
	// cache; a non-nil return withholds the payload. Called under the
	// component lock: it must not call back in.
	ValidatePayload func(fields map[string]interface{}) error
}

// NewServerlessCapabilities builds the Capabilities for serverless-init: no
// full-agent metadata refresh, every submission gated by validatePayload, and
// the payload withheld entirely when disabled.
func NewServerlessCapabilities(disabled bool, validatePayload func(fields map[string]interface{}) error) *Capabilities {
	return &Capabilities{
		Disabled:                     disabled,
		SkipFullAgentMetadataRefresh: true,
		ValidatePayload:              validatePayload,
	}
}
