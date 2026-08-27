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
	// the mechanism behind the immediate-on-start-submission capability
	// (Capabilities.ImmediateSubmission): an embedder in an environment with no
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
// component's owners as two well-motivated knobs instead of an `if serverless`
// branch.
type Capabilities struct {
	// CrossProcessEnrichment enables the payload-enrichment tier that fetches
	// configuration from the other agent processes (security/process/trace/
	// system-probe) over IPC/localhost. It is on for the full agent, where those
	// processes exist to query, and off in an environment where they do not run
	// (so the fetches would only fail, add semantic noise, and risk
	// dereferencing a nil IPC client).
	CrossProcessEnrichment bool
	// ImmediateSubmission requests a synchronous payload submission at startup
	// (see Component.Submit) instead of relying solely on the metadata runner's
	// delayed first collection. It is off for the full agent, whose 60s first-run
	// delay orders inventory after host metadata to avoid a backend
	// host-creation race, and on in an environment with no host-metadata pipeline
	// (no such race, and a short-lived process may exit before the runner fires).
	ImmediateSubmission bool
	// PayloadUUID overrides the payload's uuid. Empty means use the cached host
	// machine GUID (uuid.GetUUID()), which is correct for a host-bound agent but
	// meaningless across the ephemeral, per-process containers of a hostless
	// environment; such an embedder supplies a per-process uuid here.
	PayloadUUID string
}

// NewServerlessCapabilities builds the Capabilities that configure the
// inventoryagent component for serverless-init, a hostless single-process
// environment:
//   - CrossProcessEnrichment is false: no other agent processes run alongside
//     serverless-init, so there is nothing to query.
//   - ImmediateSubmission is true: there is no host-metadata pipeline and thus
//     no host-creation race, and the process may be very short-lived, so the
//     first payload is enqueued synchronously at startup rather than left to
//     the runner goroutine.
//   - PayloadUUID is a per-process uuid: serverless containers do not share the
//     host machine GUID that uuid.GetUUID() returns.
func NewServerlessCapabilities(processUUID string) *Capabilities {
	return &Capabilities{
		CrossProcessEnrichment: false,
		ImmediateSubmission:    true,
		PayloadUUID:            processUUID,
	}
}
