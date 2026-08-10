// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package workloadbalancingimpl implements a component to generate the 'workload_balancing_metadata' metadata payload for inventory.
package workloadbalancingimpl

import (
	"encoding/json"
	"maps"
	"net/http"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/metadata/internal/util"
	workloadbalancingcomp "github.com/DataDog/datadog-agent/comp/workloadbalancing/def"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	httputils "github.com/DataDog/datadog-agent/pkg/util/http"
)

type workloadBalancingMetadata struct {
	Enabled bool `json:"enabled"`
	// Groups maps a workload balancing group to the state this Agent holds for it
	Groups map[string]string `json:"groups"`
}

// Payload handles the JSON unmarshalling of the metadata payload
type Payload struct {
	Hostname  string                     `json:"hostname"`
	Timestamp int64                      `json:"timestamp"`
	Metadata  *workloadBalancingMetadata `json:"workload_balancing_metadata"`
}

// MarshalJSON serialization a Payload to JSON
func (p *Payload) MarshalJSON() ([]byte, error) {
	type PayloadAlias Payload
	return json.Marshal((*PayloadAlias)(p))
}

type workloadbalancingimpl struct {
	util.InventoryPayload

	conf              config.Component
	log               log.Component
	m                 sync.Mutex
	data              *workloadBalancingMetadata
	hostname          string
	workloadBalancing workloadbalancingcomp.Component
}

func (i *workloadbalancingimpl) refreshMetadata() {
	isEnabled := i.workloadBalancing.Enabled()

	if !isEnabled {
		i.data = nil
		return
	}

	groups := make(map[string]string)
	for groupID, state := range i.workloadBalancing.GetGroupStates() {
		groups[groupID] = string(state)
	}

	i.data = &workloadBalancingMetadata{
		Enabled: isEnabled,
		Groups:  groups,
	}
}

func (i *workloadbalancingimpl) getPayload() marshaler.JSONMarshaler {
	i.m.Lock()
	defer i.m.Unlock()

	i.refreshMetadata()

	return &Payload{
		Hostname:  i.hostname,
		Timestamp: time.Now().UnixNano(),
		Metadata:  i.getDataCopy(),
	}
}

func (i *workloadbalancingimpl) writePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	// GetAsJSON already return scrubbed data
	scrubbed, err := i.GetAsJSON()
	if err != nil {
		httputils.SetJSONError(w, err, 500)
		return
	}
	w.Write(scrubbed)
}

// Get returns a copy of the workload balancing metadata, refreshed live. Useful to be incorporated
// in the status page.
//
// It recomputes the metadata on every call rather than returning the value cached by the periodic
// inventory collector (which can be up to MaxInterval stale), since group assignments change
// between submissions and the status page is expected to show the current one.
func (i *workloadbalancingimpl) Get() *workloadBalancingMetadata {
	i.m.Lock()
	defer i.m.Unlock()
	i.refreshMetadata()
	return i.getDataCopy()
}

func (i *workloadbalancingimpl) getDataCopy() *workloadBalancingMetadata {
	if i.data == nil {
		return nil
	}
	dataCopy := *i.data
	dataCopy.Groups = maps.Clone(i.data.Groups)
	return &dataCopy
}
