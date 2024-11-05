// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package remoteagentimpl implements the remoteagent component interface
package remoteagentimpl

import (
	"context"
	"crypto/tls"
	"fmt"
	"sync"
	"time"

	flarebuilder "github.com/DataDog/datadog-agent/comp/core/flare/builder"
	flaretypes "github.com/DataDog/datadog-agent/comp/core/flare/types"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagent/def"
	raproto "github.com/DataDog/datadog-agent/comp/core/remoteagent/proto"
	remoteagentStatus "github.com/DataDog/datadog-agent/comp/core/remoteagent/status"
	util "github.com/DataDog/datadog-agent/comp/core/remoteagent/util"
	"github.com/DataDog/datadog-agent/comp/core/status"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	ddgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const remoteAgentIdleTimeout = 30 * time.Second
const remoteAgentQueryTimeout = 5 * time.Second
const remoteAgentRecommendedRefreshIntervalSecond = uint32(10)

// Requires defines the dependencies for the remoteagent component
type dependencies struct {
	Lifecycle fx.Lifecycle
}

// Provides defines the output of the remoteagent component
type Provides struct {
	Comp          remoteagent.Component
	FlareProvider flaretypes.Provider
	Status        status.InformationProvider
}

// NewComponent creates a new remoteagent component
func NewComponent(deps dependencies) Provides {
	c := newRemoteAgent(deps)

	return Provides{
		Comp:          c,
		FlareProvider: flaretypes.NewProvider(c.(*RemoteAgent).fillFlare),
		Status:        status.NewInformationProvider(remoteagentStatus.GetProvider(c)),
	}
}

func newRemoteAgent(deps dependencies) remoteagent.Component {
	shutdownChan := make(chan struct{})
	comp := &RemoteAgent{
		agentMap:     make(map[string]*remoteAgentDetails),
		shutdownChan: shutdownChan,
	}

	deps.Lifecycle.Append(fx.Hook{
		OnStart: func(context.Context) error {
			go comp.start()
			return nil
		},
		OnStop: func(context.Context) error {
			shutdownChan <- struct{}{}
			return nil
		}})

	return comp
}

// RemoteAgent is the main registry for remote agents. It tracks which remote agents are currently registered, when
// they were last seen, and handles collecting status and flare data from them on request.
type RemoteAgent struct {
	agentMap     map[string]*remoteAgentDetails
	agentMapMu   sync.Mutex
	shutdownChan chan struct{}
}

// RegisterRemoteAgent registers a remote agent with the registry.
//
// If the remote agent is not present in the registry, it is added. If a remote agent with the same ID is already
// present, the API endpoint and display name are checked: if they are the same, then the "last seen" time of the remote
// agent is updated, otherwise the remote agent is removed and replaced with the incoming one.
func (ra *RemoteAgent) RegisterRemoteAgent(registration *remoteagent.RegistrationData) (uint32, error) {
	ra.agentMapMu.Lock()
	defer ra.agentMapMu.Unlock()

	// If the remote agent is already registered, then we might be dealing with an update (i.e., periodic check-in) or a
	// brand new remote agent. As the agent ID may very well not be a unique ID every single time (it could always just
	// be `process-agent` or what have you), we differentiate between the two scenarios by checking the human name and
	// API endpoint give to us.
	//
	// If either of them are different, then we remove the old remote agent and add the new one. If they're the same,
	// then we just update the last seen time and move on.
	agentID := util.SanitizeAgentID(registration.AgentID)
	entry, ok := ra.agentMap[agentID]
	if !ok {
		// We've got a brand new remote agent, so do a sanity check by trying to connect to their gRPC endpoint and if
		// that works, add them to the registry.
		details, err := newRemoteAgentDetails(registration)
		if err != nil {
			return 0, err
		}

		log.Infof("Remote agent '%s' registered.", agentID)
		ra.agentMap[agentID] = details
		return remoteAgentRecommendedRefreshIntervalSecond, nil
	}

	// We already have an entry for this remote agent, so check if we need to update the gRPC client, and then update
	// the other bits.
	if entry.apiEndpoint != registration.APIEndpoint {
		entry.apiEndpoint = registration.APIEndpoint

		client, err := newRemoteAgentClient(registration)
		if err != nil {
			return 0, err
		}
		entry.client = client
	}

	entry.displayName = registration.DisplayName
	entry.lastSeen = time.Now()

	return remoteAgentRecommendedRefreshIntervalSecond, nil
}

// Start starts the remote agent registry, which periodically checks for idle remote agents and deregisters them.
func (ra *RemoteAgent) start() {
	go func() {
		log.Info("Remote Agent registry started.")

		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ra.shutdownChan:
				log.Info("Remote Agent registry stopped.")
				return
			case <-ticker.C:
				ra.agentMapMu.Lock()

				agentsToRemove := make([]string, 0)
				for id, details := range ra.agentMap {
					if time.Since(details.lastSeen) > remoteAgentIdleTimeout {
						agentsToRemove = append(agentsToRemove, id)
					}
				}

				for _, id := range agentsToRemove {
					delete(ra.agentMap, id)
					log.Infof("Remote agent '%s' deregistered after being idle for %s.", id, remoteAgentIdleTimeout)
				}

				ra.agentMapMu.Unlock()
			}
		}
	}()
}

// GetRegisteredAgents returns the list of registered remote agents.
func (ra *RemoteAgent) GetRegisteredAgents() []*remoteagent.RegisteredAgent {
	ra.agentMapMu.Lock()
	defer ra.agentMapMu.Unlock()

	agents := make([]*remoteagent.RegisteredAgent, 0, len(ra.agentMap))
	for _, details := range ra.agentMap {
		agents = append(agents, &remoteagent.RegisteredAgent{
			DisplayName:  details.displayName,
			LastSeenUnix: details.lastSeen.Unix(),
		})
	}

	return agents
}

// GetRegisteredAgentStatuses returns the status of all registered remote agents.
func (ra *RemoteAgent) GetRegisteredAgentStatuses() []*remoteagent.StatusData {
	ra.agentMapMu.Lock()

	agentsLen := len(ra.agentMap)
	statusMap := make(map[string]*remoteagent.StatusData, agentsLen)
	agentStatuses := make([]*remoteagent.StatusData, 0, agentsLen)

	// Return early if we have no registered remote agents.
	if agentsLen == 0 {
		ra.agentMapMu.Unlock()
		return agentStatuses
	}

	// We preload the status map with a response that indicates timeout, since we want to ensure there's an entry for
	// every registered remote agent even if we don't get a response back (whether good or bad) from them.
	for agentID, details := range ra.agentMap {
		statusMap[agentID] = &remoteagent.StatusData{
			AgentID:       agentID,
			DisplayName:   details.displayName,
			FailureReason: fmt.Sprintf("Timed out after waiting %s for response.", remoteAgentQueryTimeout),
		}
	}

	data := make(chan *remoteagent.StatusData, agentsLen)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for agentID, details := range ra.agentMap {
		displayName := details.displayName

		go func() {
			// We push any errors into "failure reason" which ends up getting shown in the status details.
			resp, err := details.client.GetStatusDetails(ctx, &pb.GetStatusDetailsRequest{}, grpc.WaitForReady(true))
			if err != nil {
				data <- &remoteagent.StatusData{
					AgentID:       agentID,
					DisplayName:   displayName,
					FailureReason: fmt.Sprintf("Failed to query for status: %v", err),
				}
				return
			}

			data <- raproto.ProtobufToStatusData(agentID, displayName, resp)
		}()
	}

	ra.agentMapMu.Unlock()

	timeout := time.After(remoteAgentQueryTimeout)
	responsesRemaining := agentsLen

collect:
	for {
		select {
		case statusData := <-data:
			statusMap[statusData.AgentID] = statusData
			responsesRemaining--
		case <-timeout:
			break collect
		default:
			if responsesRemaining == 0 {
				break collect
			}
		}
	}

	// Migrate the final status data from the map into our slice, for easier consumption.
	for _, statusData := range statusMap {
		agentStatuses = append(agentStatuses, statusData)
	}

	return agentStatuses
}

func (ra *RemoteAgent) fillFlare(builder flarebuilder.FlareBuilder) error {
	ra.agentMapMu.Lock()

	agentsLen := len(ra.agentMap)
	flareMap := make(map[string]*remoteagent.FlareData, agentsLen)

	// Return early if we have no registered remote agents.
	if agentsLen == 0 {
		ra.agentMapMu.Unlock()
		return nil
	}

	data := make(chan *remoteagent.FlareData, agentsLen)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for agentID, details := range ra.agentMap {
		go func() {
			// We push any errors into "failure reason" which ends up getting shown in the status details.
			resp, err := details.client.GetFlareFiles(ctx, &pb.GetFlareFilesRequest{}, grpc.WaitForReady(true))
			if err != nil {
				log.Warnf("Failed to query remote agent '%s' for flare data: %v", agentID, err)
				data <- nil
				return
			}

			data <- raproto.ProtobufToFlareData(agentID, resp)
		}()
	}

	ra.agentMapMu.Unlock()

	timeout := time.After(remoteAgentQueryTimeout)
	responsesRemaining := agentsLen

collect:
	for {
		select {
		case flareData := <-data:
			flareMap[flareData.AgentID] = flareData
			responsesRemaining--
		case <-timeout:
			break collect
		default:
			if responsesRemaining == 0 {
				break collect
			}
		}
	}

	// We've collected all the flare data we can, so now we add it to the flare builder.
	for agentID, flareData := range flareMap {
		if flareData == nil {
			continue
		}

		for fileName, fileData := range flareData.Files {
			err := builder.AddFile(fmt.Sprintf("%s/%s", agentID, util.SanitizeFileName(fileName)), fileData)
			if err != nil {
				return fmt.Errorf("failed to add file '%s' from remote agent '%s' to flare: %w", fileName, agentID, err)
			}
		}
	}

	return nil
}

func newRemoteAgentClient(registration *remoteagent.RegistrationData) (pb.RemoteAgentClient, error) {
	tlsCreds := credentials.NewTLS(&tls.Config{
		InsecureSkipVerify: true,
	})

	conn, err := grpc.NewClient(registration.APIEndpoint,
		grpc.WithTransportCredentials(tlsCreds),
		grpc.WithPerRPCCredentials(ddgrpc.NewBearerTokenAuth(registration.AuthToken)),
		// Set on the higher side to account for the fact that flare file data could be larger than the default 4MB limit.
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024*1024)),
	)
	if err != nil {
		return nil, err
	}

	return pb.NewRemoteAgentClient(conn), nil
}

type remoteAgentDetails struct {
	lastSeen    time.Time
	displayName string
	apiEndpoint string
	client      pb.RemoteAgentClient
}

func newRemoteAgentDetails(registration *remoteagent.RegistrationData) (*remoteAgentDetails, error) {
	client, err := newRemoteAgentClient(registration)
	if err != nil {
		return nil, err
	}

	return &remoteAgentDetails{
		displayName: registration.DisplayName,
		apiEndpoint: registration.APIEndpoint,
		client:      client,
		lastSeen:    time.Now(),
	}, nil
}
