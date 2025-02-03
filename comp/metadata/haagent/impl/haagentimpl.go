// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package haagentimpl implements a component to generate the 'ha_agent_metadata' metadata payload for inventory.
package haagentimpl

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	haagentcomp "github.com/DataDog/datadog-agent/comp/haagent/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

type haAgentMetadata = map[string]interface{}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname  string          `json:"hostname"`
	Timestamp int64           `json:"timestamp"`
	Metadata  haAgentMetadata `json:"ha_agent_metadata"`
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

type haagentimpl struct {
	util.InventoryPayload

	conf     config.Component
	log      log.Component
	m        sync.Mutex
	data     haAgentMetadata
	hostname string
	haAgent  haagentcomp.Component
}

func (i *haagentimpl) refreshMetadata() {
	isEnabled := i.haAgent.Enabled()

	if !isEnabled {
		i.log.Infof("HA Agent Metadata unavailable as HA Agent is disabled")
		i.data = nil
		return
	}

	i.data["enabled"] = isEnabled
	i.data["state"] = string(i.haAgent.GetState())
}

func (i *haagentimpl) getPayload() marshaler.JSONMarshaler {
	i.m.Lock()
	defer i.m.Unlock()

	i.refreshMetadata()

	return &Payload{
		Hostname:  i.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  i.getDataCopy(),
	}
}

func (i *haagentimpl) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := i.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

// Get returns a copy of the agent metadata. Useful to be incorporated in the status page.
func (i *haagentimpl) Get() haAgentMetadata {
	i.m.Lock()
	defer i.m.Unlock()
	return i.getDataCopy()
}

func (i *haagentimpl) getDataCopy() haAgentMetadata {
	data := haAgentMetadata{}
	maps.Copy(data, i.data)
	return data
}
