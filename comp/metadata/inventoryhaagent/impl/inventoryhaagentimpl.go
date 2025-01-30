// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package inventoryhaagentimpl implements a component to generate the 'ha_agent_metadata' metadata payload for inventory.
package inventoryhaagentimpl

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
	"github.com/DataDog/datadog-agent/pkg/util/uuid"
)

type haAgentMetadata = map[string]interface{}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname  string          `json:"hostname"`
	Timestamp int64           `json:"timestamp"`
	Metadata  haAgentMetadata `json:"ha_agent_metadata"`
	UUID      string          `json:"uuid"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

// SplitPayload implements marshaler.AbstractMarshaler#SplitPayload.
//
// In this case, the payload can't be split any further.
func (p *Payload) SplitPayload(_ int) ([]marshaler.AbstractMarshaler, error) {
	return nil, fmt.Errorf("could not split inventories agent payload any more, payload is too big for intake")
}

type inventoryhaagentimpl struct {
	util.InventoryPayload

	conf     config.Component
	log      log.Component
	m        sync.Mutex
	data     haAgentMetadata
	hostname string
	haAgent  haagent.Component
}

func (i *inventoryhaagentimpl) refreshMetadata() {
	isEnabled := i.haAgent.Enabled()

	if !isEnabled {
		i.log.Infof("HA Agent Metadata unavailable as HA Agent is disabled")
		i.data = nil
		return
	}

	i.data["enabled"] = isEnabled
	i.data["state"] = string(i.haAgent.GetState())
}

func (i *inventoryhaagentimpl) getPayload() marshaler.JSONMarshaler {
	i.m.Lock()
	defer i.m.Unlock()

	i.refreshMetadata()

	// Create a static scrubbed copy of agentMetadata for the payload
	data := copyAndScrub(i.data)

	return &Payload{
		Hostname:  i.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  data,
		UUID:      uuid.GetUUID(),
	}
}

func (i *inventoryhaagentimpl) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := i.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

// Get returns a copy of the agent metadata. Useful to be incorporated in the status page.
func (i *inventoryhaagentimpl) Get() haAgentMetadata {
	i.m.Lock()
	defer i.m.Unlock()

	data := haAgentMetadata{}
	for k, v := range i.data {
		data[k] = v
	}
	return data
}
