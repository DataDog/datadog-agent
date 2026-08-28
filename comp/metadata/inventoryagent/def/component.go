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
	// Submit synchronously builds a payload and enqueues it for submission now,
	// bypassing the metadata runner's first-run delay and interval gating. It is
	// an embedder in an environment with no
	// host-metadata pipeline has no host-creation race to order around, so it can
	// enqueue the first payload immediately rather than waiting for the runner
	// goroutine, which may never be scheduled in a very short-lived process.
	Submit()
}

// Capabilities lets an embedding binary adapt the inventoryagent component for
// an environment that diverges from the standard full-agent one (currently:
// serverless-init). It is supplied as an optional fx dependency; binaries that
// do not provide it get standard full-agent behavior.
//
// Each field is a neutral capability tied to an environmental property, not to
// "serverless": the zero value is exactly full-agent behavior, and each
// non-zero value is justified by a property of the embedding environment rather
// than by a product name. This keeps the divergence legible to the inventory
// component's owners as well-motivated knobs instead of an `if serverless`
// branch.
//
// Fields are named for the divergence, not the full-agent behavior, so the Go
// zero value equals full-agent behavior: a binary that supplies no Capabilities
// (or an empty one) gets the standard agent.
type Capabilities struct {
	// SkipCrossProcessEnrichment turns off the payload-enrichment tier that
	// fetches configuration from the other agent processes (security/process/
	// trace/system-probe) over IPC/localhost. The full agent leaves it false
	// (enrichment on), where those processes exist to query. An environment where
	// they do not run sets it true (so the fetches would only fail, add semantic
	// noise, and risk dereferencing a nil IPC client).
	SkipCrossProcessEnrichment bool
	// PayloadUUID overrides the payload's uuid. Empty means use the cached host
	// machine GUID (uuid.GetUUID()), which is correct for a host-bound agent but
	// meaningless across the ephemeral, per-process containers of a hostless
	// environment; such an embedder supplies a per-process uuid here.
	PayloadUUID string
}

// NewServerlessCapabilities builds the Capabilities that configure the
// inventoryagent component for serverless-init, a hostless single-process
// environment:
//   - SkipCrossProcessEnrichment is true: no other agent processes run alongside
//     serverless-init, so there is nothing to query.
//   - PayloadUUID is a per-process uuid: serverless containers do not share the
//     host machine GUID that uuid.GetUUID() returns.
func NewServerlessCapabilities(processUUID string) *Capabilities {
	return &Capabilities{
		SkipCrossProcessEnrichment: true,
		PayloadUUID:                processUUID,
	}
}
