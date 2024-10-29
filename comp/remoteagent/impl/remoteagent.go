// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package remoteagentimpl implements the remoteagent component interface
package remoteagentimpl

import (
	"context"
	"fmt"
	"strings"
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

func (ra *RemoteAgent) RegisterRemoteAgent(agentId string) (remoteagent.StatusRequests, remoteagent.FlareRequests, error) {
	ra.agentMapMu.Lock()
	defer ra.agentMapMu.Unlock()

	if _, ok := ra.agentMap[agentId]; ok {
		return nil, nil, fmt.Errorf("agent ID %s already registered", agentId)
	}

	statusRequests := make(chan *remoteagent.Request[*remoteagent.StatusData])
	flareRequests := make(chan *remoteagent.Request[*remoteagent.FlareData])

	ra.agentMap[agentId] = &agentDetails{
		statusRequestsOut: statusRequests,
		flareRequestsOut:  flareRequests,
	}

	log.Infof("Remote agent '%s' registered.", agentId)

	return statusRequests, flareRequests, nil
}

func (ra *RemoteAgent) DeregisterRemoteAgent(agentId string) {
	ra.agentMapMu.Lock()
	defer ra.agentMapMu.Unlock()

	delete(ra.agentMap, agentId)

	log.Infof("Remote agent '%s' deregistered.", agentId)
}

// / GetAgentStatusMap returns the status of all registered remote agents.
func (ra *RemoteAgent) GetAgentStatusMap() map[string]*remoteagent.StatusData {
	ra.agentMapMu.Lock()

	agentsLen := len(ra.agentMap)
	statusMap := make(map[string]*remoteagent.StatusData, agentsLen)

	// Return early if we have no registered remote agents.
	if agentsLen == 0 {
		return statusMap
	}

	// Set a default value for each remote agent entry, which will be overridden if we actually do get a response back.
	for agentId := range ra.agentMap {
		statusMap[agentId] = nil
	}

	log.Tracef("Requesting status data from %d remote agents.", agentsLen)

	// Create a response channel that all connected remote agents will send their response to. We'll wait until we
	// either receive all of the expected responses or we hit our timeout.
	expectedResponses := 0
	responses := make(chan *remoteagent.StatusData, agentsLen)

	context, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	request := remoteagent.NewRequest(context, responses)

	for _, agent := range ra.agentMap {
		select {
		case agent.statusRequestsOut <- request:
			expectedResponses += 1
		default:
			log.Tracef("Remote agent '%s' is not ready to receive status requests (still pending from previous request?)", agent)
		}
	}

	ra.agentMapMu.Unlock()

	log.Tracef("Waiting for status responses from %d remote agents.", expectedResponses)

collect:
	for {
		select {
		case response := <-responses:
			log.Tracef("Received status response from remote agent '%s'.", response.AgentId)
			statusMap[response.AgentId] = response
			expectedResponses -= 1
		case <-context.Done():
			log.Tracef("Timed out waiting for status responses from %d remote agents.", expectedResponses-len(statusMap))
			break collect
		default:
			if expectedResponses == 0 {
				break collect
			}
		}
	}

	for agentId, statusData := range statusMap {
		if statusData == nil {
			log.Warnf("Remote agent '%s' did not generate status data in time.", agentId)
		}
	}

	return statusMap
}

func (ra *RemoteAgent) fillFlare(builder flarebuilder.FlareBuilder) error {
	ra.agentMapMu.Lock()

	agentsLen := len(ra.agentMap)

	// Return early if we have no registered remote agents.
	if agentsLen == 0 {
		return nil
	}

	flareMap := make(map[string]*remoteagent.FlareData, agentsLen)
	for agentId := range ra.agentMap {
		flareMap[agentId] = nil
	}

	log.Tracef("Requesting flare data from %d remote agents.", agentsLen)

	// Create a response channel that all connected remote agents will send their response to. We'll wait until we
	// either receive all of the expected responses or we hit our timeout.
	expectedResponses := 0
	responses := make(chan *remoteagent.FlareData, agentsLen)

	context, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	request := remoteagent.NewRequest(context, responses)

	for _, agent := range ra.agentMap {
		select {
		case agent.flareRequestsOut <- request:
			expectedResponses += 1
		default:
			log.Tracef("Remote agent '%s' is not ready to receive flare requests (still pending from previous request?)", agent)
		}
	}

	ra.agentMapMu.Unlock()

	log.Tracef("Waiting for flare responses from %d remote agents.", expectedResponses)

collect:
	for {
		select {
		case response := <-responses:
			log.Tracef("Received status response from remote agent '%s'.", response.AgentId)
			flareMap[response.AgentId] = response
			expectedResponses -= 1
		case <-context.Done():
			log.Tracef("Timed out waiting for status responses from %d remote agents.", expectedResponses-len(flareMap))
			break collect
		default:
			if expectedResponses == 0 {
				break collect
			}
		}
	}

	for agentId, flareData := range flareMap {
		if flareData != nil {
			for flareFile, fileData := range flareData.Files {
				if err := builder.AddFile(fmt.Sprintf("remote-agents/%s/%s", sanitizeAgentId(agentId), flareFile), fileData); err != nil {
					return err
				}
			}
		} else {
			log.Warnf("Remote agent '%s' did not generate flare data in time.", agentId)
			continue
		}
	}

	return nil
}

type agentDetails struct {
	statusRequestsOut chan<- *remoteagent.Request[*remoteagent.StatusData]
	flareRequestsOut  chan<- *remoteagent.Request[*remoteagent.FlareData]
}

func sanitizeAgentId(rawAgentId string) string {
	// TODO: This really needs to do something more like... take everything non-alphanumeric/underscores/hyphens and
	// replace those characters with an underscore/hyphen.
	//
	// Right now it just does the right thing for values being used in local testing.
	cleanedAgentId := strings.ReplaceAll(rawAgentId, " ", "-")
	return strings.ToLower(cleanedAgentId)
}
