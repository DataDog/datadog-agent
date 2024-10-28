// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package remoteagentimpl implements the remoteagent component interface
package remoteagentimpl

import (
	"fmt"
	"sync"
	"time"

	flarebuilder "github.com/DataDog/datadog-agent/comp/core/flare/builder"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	"github.com/DataDog/datadog-agent/comp/core/status"
	remoteagent "github.com/DataDog/datadog-agent/comp/remoteagent/def"
	remoteagentStatus "github.com/DataDog/datadog-agent/comp/remoteagent/status"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the dependencies for the remoteagent component
type dependencies struct {
}

// Provides defines the output of the remoteagent component
type provides struct {
	Comp          remoteagent.Component
	FlareProvider flaretypes.Provider
	Status        status.InformationProvider
}

// NewComponent creates a new remoteagent component
func NewComponent(deps dependencies) provides {
	c := newRemoteAgent()

	return provides{
		Comp:          c,
		FlareProvider: flaretypes.NewProvider(c.(*RemoteAgent).fillFlare),
		Status:        status.NewInformationProvider(remoteagentStatus.GetProvider(c)),
	}
}

func newRemoteAgent() remoteagent.Component {
	return &RemoteAgent{
		agentMap: make(map[string]*agentDetails),
	}
}

type RemoteAgent struct {
	agentMap   map[string]*agentDetails
	agentMapMu sync.Mutex
}

func (ra *RemoteAgent) RegisterRemoteAgent(agentID string) (remoteagent.StatusRequests, remoteagent.FlareRequests, error) {
	ra.agentMapMu.Lock()
	defer ra.agentMapMu.Unlock()

	if _, ok := ra.agentMap[agentID]; ok {
		return nil, nil, fmt.Errorf("agent ID %s already registered", agentID)
	}

	statusRequests := make(chan remoteagent.StatusRequest)
	flareRequests := make(chan remoteagent.FlareRequest)

	ra.agentMap[agentID] = &agentDetails{
		statusRequestsOut: statusRequests,
		flareRequestsOut:  flareRequests,
	}

	return statusRequests, flareRequests, nil
}

func (ra *RemoteAgent) DeregisterRemoteAgent(agentID string) {
	ra.agentMapMu.Lock()
	defer ra.agentMapMu.Unlock()

	delete(ra.agentMap, agentID)
}

func (ra *RemoteAgent) GetAgentStatusMap() map[string]*remoteagent.StatusData {
	ra.agentMapMu.Lock()
	defer ra.agentMapMu.Unlock()

	// Create a channel that we'll send to each remote agent's gRPC handler, and we wait for either all remote agents to
	// respond or for our timeout to expire. Remote agents that didn't respond by the timeout will be marked as having
	// timed out in their status details.
	agentsLen := len(ra.agentMap)
	statusMap := make(map[string]*remoteagent.StatusData, agentsLen)

	// Return early if we have no registered remote agents.
	if agentsLen == 0 {
		return statusMap
	}

	log.Infof("Requesting status from %d remote agents", agentsLen)

	expectedResponses := 0
	responses := make(chan *remoteagent.StatusData)
	for _, agent := range ra.agentMap {
		select {
		case agent.statusRequestsOut <- responses:
			expectedResponses += 1
		default:
			log.Debugf("Remote agent %s is not ready to receive status requests (still pending from previous request?)", agent)
		}
	}

	log.Infof("Waiting for status responses from %d remote agents", expectedResponses)

	timeout := time.NewTicker(5 * time.Second)

collect:
	for {
		select {
		case response := <-responses:
			log.Infof("Received status response from agent %s", response.AgentId)
			statusMap[response.AgentId] = response
			if len(statusMap) == expectedResponses {
				log.Infof(("Received all expected status responses, returning"))
				break collect
			}
		case <-timeout.C:
			log.Infof("Timed out waiting for status responses from %d remote agents", expectedResponses-len(statusMap))
			break collect
		}
	}

	// Mark any remote agents that didn't respond in time as timed out.
	for agentId := range ra.agentMap {
		if _, ok := statusMap[agentId]; !ok {
			statusMap[agentId] = &remoteagent.StatusData{
				AgentId:       agentId,
				FailureReason: "Failed to respond within 5 seconds.",
			}
		}
	}

	return statusMap
}

func (ra *RemoteAgent) fillFlare(builder flarebuilder.FlareBuilder) error {
	ra.agentMapMu.Lock()
	defer ra.agentMapMu.Unlock()

	// Create a channel that we'll send to each remote agent's gRPC handler, and we wait for either all remote agents to
	// respond or for our timeout to expire. Remote agents that didn't respond by the timeout will be logged.
	agentsLen := len(ra.agentMap)
	if agentsLen == 0 {
		return nil
	}

	flareMap := make(map[string]*remoteagent.FlareData, agentsLen)
	responses := make(chan *remoteagent.FlareData)
	for _, agent := range ra.agentMap {
		agent.flareRequestsOut <- responses
	}

	timeout := time.NewTicker(5 * time.Second)

collect:
	for {
		select {
		case response := <-responses:
			flareMap[response.AgentId] = response
			if len(flareMap) == agentsLen {
				break collect
			}
		case <-timeout.C:
			break collect
		}
	}

	for agentId := range ra.agentMap {
		if agentFlareData, ok := flareMap[agentId]; ok {
			for flareFile, fileData := range agentFlareData.Files {
				if err := builder.AddFile(fmt.Sprintf("remote-agents/%s", flareFile), fileData); err != nil {
					return err
				}
			}
		} else {
			log.Warnf("Remote agent %s did not generate flare data in time.", agentId)
			continue
		}
	}

	return nil
}

type agentDetails struct {
	statusRequestsOut chan<- remoteagent.StatusRequest
	flareRequestsOut  chan<- remoteagent.FlareRequest
}
