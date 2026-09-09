// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistryimpl

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mdlayher/vsock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	"github.com/google/uuid"

	remoteagentregistry "github.com/DataDog/datadog-agent/comp/core/remoteagentregistry/def"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"

	ddgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

type remoteAgentServiceName = string

// StatusServiceName is the service name for remote agent status provider
const StatusServiceName = "datadog.remoteagent.status.v1.StatusProvider"

// FlareServiceName is the service name for remote agent flare provider
const FlareServiceName = "datadog.remoteagent.flare.v1.FlareProvider"

// TelemetryServiceName is the service name for remote agent telemetry provider
const TelemetryServiceName = "datadog.remoteagent.telemetry.v1.TelemetryProvider"

type remoteAgentClient struct {
	// agent variables
	remoteagentregistry.RegisteredAgent

	// health tracking
	unhealthyReason error      // non-nil marks agent for removal during next cleanup cycle
	unhealthyMu     sync.Mutex // guards unhealthyReason

	// gRPC relative
	pb.FlareProviderClient
	pb.StatusProviderClient
	pb.TelemetryProviderClient
	services []remoteAgentServiceName
	conn     *grpc.ClientConn
}

func (ra *remoteAgentRegistry) newRemoteAgentClient(registration *remoteagentregistry.RegistrationData) (*remoteAgentClient, error) {
	if strings.TrimSpace(registration.AgentDisplayName) == "" {
		return nil, errors.New("remote agent display name must not be empty or whitespace-only")
	}
	sanitizedDisplayName := sanitizeString(registration.AgentDisplayName)

	target, dialOpts, err := resolveDialTarget(registration.APIEndpointURI, ra.ipc.GetTLSClientConfig())
	if err != nil {
		return nil, err
	}

	dialOpts = append(dialOpts,
		grpc.WithPerRPCCredentials(ddgrpc.NewBearerTokenAuth(ra.ipc.GetAuthToken())),
		// Set on the higher side to account for the fact that flare file data could be larger than the default 4MB limit.
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64*1024*1024)),
	)

	conn, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		return nil, err
	}

	client := &remoteAgentClient{
		RegisteredAgent: remoteagentregistry.RegisteredAgent{
			Flavor:               registration.AgentFlavor,
			DisplayName:          registration.AgentDisplayName,
			SanitizedDisplayName: sanitizedDisplayName,
			PID:                  registration.AgentPID,
			LastSeen:             time.Now(),
			SessionID:            uuid.New().String(),
		},
		// gRPC relative
		conn:                    conn,
		StatusProviderClient:    pb.NewStatusProviderClient(conn),
		FlareProviderClient:     pb.NewFlareProviderClient(conn),
		TelemetryProviderClient: pb.NewTelemetryProviderClient(conn),
	}

	client.services = registration.Services

	return client, nil
}

// resolveDialTarget translates a remote agent's advertised api_endpoint_uri into
// a gRPC dial target plus the dial options to use for the connection.
//
// Supported schemes (defined in datadog/remoteagent/remoteagent.proto):
//   - "unix:///path"      — UDS, TLS preserved (filesystem perms gate access, TLS protects on-wire bytes).
//   - "https://host:port" — TCP with TLS.
//   - "vsock://cid:port"  — AF_VSOCK, TLS preserved (used on kata/microVM clusters where the remote
//     agent runs in a separate guest VM from the core agent).
func resolveDialTarget(endpointURI string, tlsConfig *tls.Config) (string, []grpc.DialOption, error) {
	tlsCreds := credentials.NewTLS(tlsConfig)

	scheme, rest, hasScheme := strings.Cut(endpointURI, "://")
	if !hasScheme {
		// No scheme: backwards-compat path, treat as host:port over TLS.
		return endpointURI, []grpc.DialOption{grpc.WithTransportCredentials(tlsCreds)}, nil
	}

	switch strings.ToLower(scheme) {
	case "unix":
		// gRPC's built-in unix resolver expects the original "unix://" target string.
		return endpointURI, []grpc.DialOption{grpc.WithTransportCredentials(tlsCreds)}, nil
	case "https":
		return rest, []grpc.DialOption{grpc.WithTransportCredentials(tlsCreds)}, nil
	case "vsock":
		cid, port, err := parseVSockEndpoint(rest)
		if err != nil {
			return "", nil, fmt.Errorf("invalid vsock api_endpoint_uri %q: %w", endpointURI, err)
		}
		dialer := func(_ context.Context, _ string) (net.Conn, error) {
			return vsock.Dial(cid, port, &vsock.Config{})
		}
		// The target's host is otherwise unused for dialing (fully delegated to the context
		// dialer above), but it still drives the gRPC :authority and thus the TLS ServerName;
		// keep it "localhost" to match the SANs on the Agent IPC cert (mirrors the existing
		// vsock dial in pkg/util/grpc/agent_client.go).
		return net.JoinHostPort("localhost", strconv.Itoa(int(port))), []grpc.DialOption{
			grpc.WithTransportCredentials(tlsCreds),
			grpc.WithContextDialer(dialer),
		}, nil
	default:
		return "", nil, fmt.Errorf("unsupported api_endpoint_uri scheme %q (expected one of: unix, https, vsock)", scheme)
	}
}

// parseVSockEndpoint parses a "cid:port" vsock host part (as found after the "vsock://" scheme)
// into its numeric context ID and port.
func parseVSockEndpoint(hostPort string) (cid uint32, port uint32, err error) {
	cidStr, portStr, err := net.SplitHostPort(hostPort)
	if err != nil {
		return 0, 0, err
	}

	cid64, err := strconv.ParseUint(cidStr, 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid context ID %q: %w", cidStr, err)
	}

	port64, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid port %q: %w", portStr, err)
	}

	return uint32(cid64), uint32(port64), nil
}

// close closes the remote agent client and its connection
func (rac *remoteAgentClient) close() error {
	return rac.conn.Close()
}

// validateSessionID extracts and validates the session_id from gRPC response metadata.
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
//   - service:  The full service name (e.g., datadog.remoteagent.status.v1.StatusProvider).
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
	queryTimeout := registry.conf.GetDuration("remote_agent.registry.query_timeout")

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
		// Snapshot the RegisteredAgent value under the lock so the goroutines
		// don't race with RefreshRemoteAgent writing LastSeen. The gRPC
		// client methods on remoteAgent use the conn, not RegisteredAgent.
		registeredAgent := remoteAgent.RegisteredAgent
		go func() {
			start := time.Now()
			defer func() {
				wg.Done()
				registry.telemetryStore.remoteAgentActionDuration.Observe(
					time.Since(start).Seconds(),
					registeredAgent.SanitizedDisplayName,
					service,
				)
			}()

			var responseHeader metadata.MD
			// We push any errors into "failure reason" which ends up getting shown in the status details.
			resp, err := grpcCall(ctx, remoteAgent, grpc.WaitForReady(true), grpc.Header(&responseHeader))

			if err != nil {
				registry.telemetryStore.remoteAgentActionError.Inc(registeredAgent.SanitizedDisplayName, service, grpcErrorMessage(err))
			} else {
				// Validate session ID if no error occurred
				if validationErr := remoteAgent.validateSessionID(responseHeader); validationErr != nil {
					// wrap error in gRPC status
					err = validationErr
					registry.telemetryStore.remoteAgentActionError.Inc(registeredAgent.SanitizedDisplayName, service, sessionIDMismatch)

					// Mark agent as unhealthy for removal during next cleanup cycle
					remoteAgent.unhealthyMu.Lock()
					remoteAgent.unhealthyReason = validationErr
					remoteAgent.unhealthyMu.Unlock()
				}
			}

			// Append the result to the result slice
			resultLock.Lock()
			resultSlice = append(resultSlice, resultProcessor(registeredAgent, resp, err))
			resultLock.Unlock()
		}()
	}

	registry.agentMapMu.Unlock()

	wg.Wait()

	return resultSlice
}

func sanitizeString(in string) string {
	return strings.ReplaceAll(strings.ToLower(in), " ", "-")
}
