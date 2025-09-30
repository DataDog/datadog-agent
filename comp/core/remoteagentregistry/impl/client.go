// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/reflection/grpc_reflection_v1"

	"github.com/google/uuid"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"

	ddgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

type remoteAgentServiceName = string

// StatusServiceName is the service name for remote agent status provider
const StatusServiceName = "datadog.remoteagent.StatusProvider"

// FlareServiceName is the service name for remote agent flare provider
const FlareServiceName = "datadog.remoteagent.FlareProvider"

// TelemetryServiceName is the service name for remote agent telemetry provider
const TelemetryServiceName = "datadog.remoteagent.TelemetryProvider"

var remoteAgentServices = map[remoteAgentServiceName]struct{}{
	StatusServiceName:    {},
	FlareServiceName:     {},
	TelemetryServiceName: {},
}

type remoteAgentClient struct {
	// agent variables
	remoteagentregistry.RegisteredAgent

	// health tracking
	unhealthy bool // marks agent for removal during next cleanup cycle

	// gRPC relative
	grpc_reflection_v1.ServerReflectionClient
	pb.FlareProviderClient
	pb.StatusProviderClient
	pb.TelemetryProviderClient
	services []remoteAgentServiceName
	conn     *grpc.ClientConn
}

func (ra *remoteAgentRegistry) newRemoteAgentClient(registration *remoteagentregistry.RegistrationData) (*remoteAgentClient, error) {
	conn, err := grpc.NewClient(registration.APIEndpointURI,
		grpc.WithTransportCredentials(credentials.NewTLS(ra.ipc.GetTLSClientConfig())),
		grpc.WithPerRPCCredentials(ddgrpc.NewBearerTokenAuth(ra.ipc.GetAuthToken())),
		// Set on the higher side to account for the fact that flare file data could be larger than the default 4MB limit.
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024*1024)),
	)
	if err != nil {
		return nil, err
	}

	client := &remoteAgentClient{
		RegisteredAgent: remoteagentregistry.RegisteredAgent{
			Flavor:               registration.AgentFlavor,
			DisplayName:          registration.AgentDisplayName,
			SanitizedDisplayName: sanitizeString(registration.AgentDisplayName),
			PID:                  registration.AgentPID,
			LastSeen:             time.Now(),
			SessionID:            uuid.New().String(),
		},
		// gRPC relative
		ServerReflectionClient:  grpc_reflection_v1.NewServerReflectionClient(conn),
		conn:                    conn,
		StatusProviderClient:    pb.NewStatusProviderClient(conn),
		FlareProviderClient:     pb.NewFlareProviderClient(conn),
		TelemetryProviderClient: pb.NewTelemetryProviderClient(conn),
	}

	// retrieve remote Agent exposed services via gRPC reflection
	services, err := client.fetchSupportedServices()
	if err != nil {
		return nil, fmt.Errorf("unable to create remoteAgent client: %w", err)
	}
	client.services = services

	return client, nil
}

func (rac *remoteAgentClient) fetchSupportedServices() ([]remoteAgentServiceName, error) {
	// Initialize the reflection
	streamCtx, streamCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer streamCancel()
	stream, err := rac.ServerReflectionInfo(streamCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to create reflection stream: %v", err)
	}

	// Discover services using the stored stream (always initialized in constructor)
	err = stream.Send(&grpc_reflection_v1.ServerReflectionRequest{
		MessageRequest: &grpc_reflection_v1.ServerReflectionRequest_ListServices{},
	})
	if err != nil {
		return nil, fmt.Errorf("unable to send reflection request: %v", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		return nil, fmt.Errorf("unable to receive reflection response: %v", err)
	}

	listServicesResp := resp.GetListServicesResponse()
	if listServicesResp == nil {
		return nil, fmt.Errorf("invalid reflection response: expected ListServicesResponse")
	}

	availableServices := []remoteAgentServiceName{}

	for _, service := range listServicesResp.Service {
		if _, ok := remoteAgentServices[service.Name]; ok {
			availableServices = append(availableServices, service.Name)
		}
	}

	return availableServices, nil
}

// validateSessionID extracts and validates the session_id from gRPC response metadata
func (rac *remoteAgentClient) validateSessionID(responseMetadata metadata.MD) error {
	sessionIDs := responseMetadata.Get("session_id")
	if len(sessionIDs) == 0 {
		return errors.New("missing session_id in response metadata")
	}

	if len(sessionIDs) > 1 {
		return errors.New("multiple session_id values in response metadata")
	}

	receivedSessionID := sessionIDs[0]
	if receivedSessionID != rac.RegisteredAgent.SessionID {
		return fmt.Errorf("session_id mismatch: expected %s, got %s", rac.RegisteredAgent.SessionID, receivedSessionID)
	}

	return nil
}

// callAgentsForService concurrently invokes a gRPC call on all registered remote agents that support a given service.
// It filters agents by service capability, applies a timeout to each call, and collects telemetry for each attempt.
// The function returns a slice of processed results, one per agent, using the provided processor function.
//
// Type Parameters:
//   - PbType:         The raw protobuf response type returned by the gRPC call.
//   - StructuredType: The processed output type produced by the processor.
//
// Parameters:
//   - registry:     The remote agent registry containing all known agents.
//   - service:  The service to query (e.g., statusProvider, flareProvider).
//   - grpcCall:   Function to perform the gRPC call for a given agent.
//   - resultProcessor:    Function to transform the gRPC response (or error) into the desired output type.
//
// Returns:
//   - []StructuredType: A slice of processed results, one per agent that supports the service.
func callAgentsForService[PbType any, StructuredType any](
	registry *remoteAgentRegistry,
	service remoteAgentServiceName,
	grpcCall func(context.Context, *remoteAgentClient, ...grpc.CallOption) (PbType, error),
	resultProcessor func(remoteagentregistry.RegisteredAgent, PbType, error) StructuredType,
) []StructuredType {
	queryTimeout := registry.conf.GetDuration("remote_agent_registry.query_timeout")

	var wg sync.WaitGroup

	registry.agentMapMu.Lock()

	filteredAgents := []*remoteAgentClient{}

	for _, remoteAgent := range registry.agentMap {
		// Skip the remoteAgent if the service is not implemented
		if !slices.Contains(remoteAgent.services, service) {
			continue
		}
		filteredAgents = append(filteredAgents, remoteAgent)
	}

	agentsLen := len(filteredAgents)
	resultSlice := make([]StructuredType, 0, agentsLen)
	var resultLock sync.Mutex

	// Return early if we have no registered remote agents.
	if agentsLen == 0 {
		registry.agentMapMu.Unlock()
		return resultSlice
	}

	// Creates a context with a one second deadline for the RPC.
	ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
	defer cancel()

	wg.Add(agentsLen)
	for _, remoteAgent := range filteredAgents {
		go func() {
			start := time.Now()
			defer func() {
				wg.Done()
				registry.telemetryStore.remoteAgentActionDuration.Observe(
					time.Since(start).Seconds(),
					remoteAgent.RegisteredAgent.SanitizedDisplayName,
					service,
				)
			}()

			var responseHeader metadata.MD
			// We push any errors into "failure reason" which ends up getting shown in the status details.
			resp, err := grpcCall(ctx, remoteAgent, grpc.WaitForReady(true), grpc.Header(&responseHeader))

			if err != nil {
				registry.telemetryStore.remoteAgentActionError.Inc(remoteAgent.RegisteredAgent.SanitizedDisplayName, service, grpcErrorMessage(err))
			} else {
				// Validate session ID if no error occurred
				if validationErr := remoteAgent.validateSessionID(responseHeader); validationErr != nil {
					// wrap error in gRPC status
					err = validationErr
					registry.telemetryStore.remoteAgentActionError.Inc(remoteAgent.RegisteredAgent.SanitizedDisplayName, service, sessionIDMismatch)

					// Mark agent as unhealthy for removal during next cleanup cycle
					remoteAgent.unhealthy = true
				}
			}

			// Append the result to the result slice
			resultLock.Lock()
			resultSlice = append(resultSlice, resultProcessor(remoteAgent.RegisteredAgent, resp, err))
			resultLock.Unlock()
		}()
	}

	registry.agentMapMu.Unlock()

	wg.Wait()

	return resultSlice
}

func sanitizeString(in string) string {
	out := []string{}
	for _, s := range strings.Split(in, " ") {
		out = append(out, strings.ToLower(s))
	}
	return strings.Join(out, "-")
}
