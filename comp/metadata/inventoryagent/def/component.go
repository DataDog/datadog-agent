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
}

// serverlessInitFlavor is the payload flavor emitted by serverless-init. It
// carries the dash intentionally and is injected only into the payload (via
// Params.Flavor). serverless-init does not change the process-global flavor
// (flavor.SetFlavor), which stays "agent": the aggregator captures the flavor
// once at construction for the datadog.<flavor>.running / .up heartbeat metric
// and service check, so renaming it process-wide would break agent-host
// identification and existing monitors.
const serverlessInitFlavor = "serverless-init"

// Params lets an embedding binary adapt the inventoryagent component for an
// environment that diverges from the standard full-agent one (currently:
// serverless-init). It is supplied as an optional fx dependency; binaries that
// do not provide it get the standard full-agent behavior.
//
// This mirrors the metadata/resources component's optional Params (see
// resourcesimpl.Params / Disabled()) and rcclient's Params: a construction-time
// struct, supplied by the binary and received with `optional:"true"`, whose
// fields select which code paths the constructor takes.
//
// Each field exists because reusing the full-agent component in serverless
// surfaced a concrete divergence:
//   - Enabled: the component's enablement is normally derived from
//     util.InventoryEnabled (inventories_enabled + enable_metadata_collection),
//     keys that are absent from the serverless config schema. A caller with its
//     own enablement gate sets this pointer to force the value.
//   - Flavor: initData normally hardwires flavor.GetFlavor() ("agent").
//     serverless must emit "serverless-init" in the payload without changing the
//     process-global flavor (which the aggregator captures for its heartbeat
//     metric), so it overrides just the payload value.
//   - UUID: getPayload normally uses uuid.GetUUID() (the cached host machine
//     GUID), which is meaningless across serverless containers. A caller can
//     supply a per-process UUID instead.
//   - ExtraFields: additional agent_metadata keys layered into every payload
//     (serverless injects its cloud-platform fields here rather than through
//     Set, so they are present on the very first payload).
//   - SkipRemoteMetadata: the full-agent payload fetches config from the
//     security/process/trace/system-probe processes over IPC/localhost. None of
//     those run alongside serverless-init, so a caller can skip that phase.
type Params struct {
	// Enabled, when non-nil, forces the component's Enabled flag instead of
	// deriving it from util.InventoryEnabled.
	Enabled *bool
	// Flavor, when non-empty, is the payload flavor value (e.g.
	// "serverless-init") used in place of flavor.GetFlavor().
	Flavor string
	// UUID, when non-empty, is the payload uuid used in place of
	// uuid.GetUUID().
	UUID string
	// ExtraFields are additional agent_metadata keys merged into every payload.
	ExtraFields map[string]interface{}
	// SkipRemoteMetadata skips fetching metadata from other agent processes
	// (security/process/trace/system-probe) when true.
	SkipRemoteMetadata bool
}

// NewServerlessParams builds the Params that configure the inventoryagent
// component for serverless-init: the serverless-init payload flavor, a
// per-process uuid, no cross-process metadata fetching, and the supplied
// enablement and extra fields. It encodes the serverless defaults the same way
// resourcesimpl.Disabled() encodes a specific resources configuration.
func NewServerlessParams(enabled bool, processUUID string, extraFields map[string]interface{}) *Params {
	return &Params{
		Enabled:            &enabled,
		Flavor:             serverlessInitFlavor,
		UUID:               processUUID,
		ExtraFields:        extraFields,
		SkipRemoteMetadata: true,
	}
}
