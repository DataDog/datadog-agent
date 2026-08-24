// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package serverlessinventoryimpl implements a component to generate the
// serverless-init inventory metadata payload.
//
// The payload envelope, scheduling, submission, and wiring reuse the shared
// inventory machinery (comp/metadata/internal/util.InventoryPayload); the
// serverless-specific fields are delegated to the injected FieldProvider,
// which is implemented in cmd/serverless-init so all cloud-platform derivation
// stays there.
package serverlessinventoryimpl

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/DataDog/datadog-agent/comp/core/config"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	runnerdef "github.com/DataDog/datadog-agent/comp/metadata/runner/def"
	serverlessinventory "github.com/DataDog/datadog-agent/comp/metadata/serverlessinventory/def"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// flavorValue is the payload flavor for serverless-init. It carries the dash
// intentionally and is injected only into the payload. We do not change the
// process-global flavor (flavor.SetFlavor), which stays "agent": the
// aggregator captures the flavor once at construction for the
// datadog.<flavor>.running / .up heartbeat metric and service check, so
// renaming it process-wide would break agent-host identification and existing
// monitors.
const flavorValue = "serverless-init"

// serverlessFieldPrefix is applied uniformly to every serverless-specific key
// in agent_metadata.
const serverlessFieldPrefix = "serverless_"

// Payload is the serverless-init inventory metadata payload. It is owned here
// (rather than reusing the inventoryagent Payload) so serverless controls the
// envelope: a per-process uuid, an empty hostname, and no shared host GUID.
type Payload struct {
	Hostname  string                 `json:"hostname"`
	Timestamp int64                  `json:"timestamp"`
	Metadata  map[string]interface{} `json:"agent_metadata"`
	UUID      string                 `json:"uuid"`
}

// MarshalJSON serializes a Payload to JSON.
func (p *Payload) MarshalJSON() ([]byte, error) {
	type payloadAlias Payload
	return json.Marshal((*payloadAlias)(p))
}

type serverlessInventory struct {
	util.InventoryPayload

	log    log.Component
	conf   config.Component
	fields serverlessinventory.FieldProvider
	uuid   string
}

// Requires defines the dependencies for the serverlessinventory component.
type Requires struct {
	Log        log.Component
	Config     config.Component
	Serializer serializer.MetricSerializer
	Fields     serverlessinventory.FieldProvider
}

// Provides defines the output of the serverlessinventory component.
type Provides struct {
	Comp          serverlessinventory.Component
	Provider      runnerdef.Provider
	FlareProvider flaretypes.Provider
}

// NewComponent creates a new serverlessinventory component.
func NewComponent(deps Requires) Provides {
	si := &serverlessInventory{
		conf:   deps.Config,
		log:    deps.Log,
		fields: deps.Fields,
		// A per-process uuid: serverless containers do not share the host
		// machine GUID that uuid.GetUUID() returns.
		uuid: uuid.New().String(),
	}
	si.InventoryPayload = util.CreateInventoryPayload(deps.Config, deps.Log, deps.Serializer, si.getPayload, "serverless-init.json")

	// CreateInventoryPayload caches its Enabled flag from util.InventoryEnabled,
	// which reads inventories_enabled (not present in the serverless config
	// schema) and does not consult any serverless-specific gate. Override it
	// here so emission is controlled by enabled() below.
	si.Enabled = enabled(deps.Config)

	return Provides{
		Comp:          si,
		Provider:      si.MetadataProvider(),
		FlareProvider: si.FlareProvider(),
	}
}

// enabled reports whether serverless-init inventory emission is on. It is
// disabled by default while the feature ramps.
//
// TODO(SVLS): gate on a serverless-scoped config key
// (DD_SERVERLESS_INIT_INVENTORY_ENABLED) once it is added to the serverless
// config schema, combined with the shared metadata-collection switch.
func enabled(_ config.Component) bool {
	return false
}

// getPayload is the PayloadGetter callback invoked by the metadata runner. The
// payload content does not change between sends, so a single successful send
// per process lifetime is sufficient.
func (si *serverlessInventory) getPayload() marshaler.JSONMarshaler {
	data := make(map[string]interface{})

	populateCoreFields(data, si.conf, flavorValue)
	addPrefixedFields(data, si.fields.GetInventoryFields())

	return &Payload{
		Hostname:  "", // empty for serverless; no host identity
		Timestamp: time.Now().UnixNano(),
		Metadata:  data,
		UUID:      si.uuid,
	}
}

// Refresh implements serverlessinventory.Component.
func (si *serverlessInventory) Refresh() {
	si.InventoryPayload.Refresh()
}

// addPrefixedFields flattens the typed Fields into data, applying the
// serverless_ prefix to each key. The Fields JSON tags are the single source
// of truth for the key names, so this stays generic and never enumerates
// fields by hand.
func addPrefixedFields(data map[string]interface{}, fields serverlessinventory.Fields) {
	raw, err := json.Marshal(fields)
	if err != nil {
		return
	}
	var flat map[string]interface{}
	if err := json.Unmarshal(raw, &flat); err != nil {
		return
	}
	for key, value := range flat {
		data[serverlessFieldPrefix+key] = value
	}
}

// populateCoreFields sets the subset of agent-metadata fields required for a
// payload to be recognized and processed as an agent-metadata payload by the
// inventory pipeline. The flavor is injected rather than read from
// flavor.GetFlavor() so serverless-init can emit its own flavor without
// changing the process-global one.
//
// These fields mirror the no-cross-process-dependency core fields the core
// agent's inventoryagent component sets. Fields that depend on cross-process
// fetchers (security/process/trace/system-probe) or config dumps must NOT be
// added here.
//
// TODO(SVLS): the install_method_* values are placeholders; wire them from the
// install-info the core agent reads.
func populateCoreFields(data map[string]interface{}, conf config.Component, flavor string) {
	data["agent_version"] = version.AgentVersion
	data["package_version"] = version.AgentPackageVersion
	data["agent_startup_time_ms"] = conf.StartTime().UnixMilli()
	data["flavor"] = flavor

	data["install_method_tool"] = "undefined"
	data["install_method_tool_version"] = ""
	data["install_method_installer_version"] = ""
}
