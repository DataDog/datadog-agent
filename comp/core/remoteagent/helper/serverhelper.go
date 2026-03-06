// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package helper implements the helper for the remoteagent component
package helper

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"

	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// UnimplementedRemoteAgentServer is a wrapper around a gRPC server that implements the RemoteAgentServer protocol.
// It takes care of the registration logic with the Core Agent.
type UnimplementedRemoteAgentServer struct {
	log    log.Component
	config config.Component

	// server infos
	agentFlavor string
	displayName string
	services    []string

	// communication components
	ipcComp         ipc.Component
	agentClient     pbcore.AgentSecureClient
	sessionID       string
	sessionIDMutex  sync.RWMutex
	agentIpcAddress string
	listener        net.Listener
	grpcServer      *grpc.Server

	// lifecycle components
	ctx                    context.Context
	cancel                 context.CancelFunc
	wg                     sync.WaitGroup
	defaultRefreshInterval time.Duration
	queryTimeout           time.Duration
}

// NewUnimplementedRemoteAgentServer creates a new unimplemented remote agent server
func NewUnimplementedRemoteAgentServer(ipcComp ipc.Component, log log.Component, config config.Component, lc compdef.Lifecycle, agentIpcAddress string, agentFlavor string, displayName string) (*UnimplementedRemoteAgentServer, error) {
	// Validate the agentFlavor and displayName
	if agentFlavor == "" {
		return nil, errors.New("agentFlavor is required")
	}
	if displayName == "" {
		return nil, errors.New("displayName is required")
	}

	// Listen on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	agentClient, err := newAgentSecureClient(ipcComp, agentIpcAddress)
	if err != nil {
		log.Errorf("failed to create agent client: %v", err)
		return nil, err
	}

	remoteAgentServer := &UnimplementedRemoteAgentServer{
		ipcComp:                ipcComp,
		log:                    log,
		config:                 config,
		agentIpcAddress:        agentIpcAddress,
		agentFlavor:            agentFlavor,
		displayName:            displayName,
		agentClient:            agentClient,
		listener:               listener,
		grpcServer:             nil,
		defaultRefreshInterval: 5 * time.Second,
		queryTimeout:           config.GetDuration("remote_agent_registry.query_timeout"),
	}

	// Initialize the gRPC server
	sessionIDInterceptor := func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		remoteAgentServer.sessionIDMutex.RLock()
		sessionID := remoteAgentServer.sessionID
		remoteAgentServer.sessionIDMutex.RUnlock()
		if sessionID == "" {
			return nil, errors.New("remote agent is not registered yet")
		}
		err = grpc.SetHeader(ctx, metadata.New(map[string]string{"session_id": sessionID}))
		if err != nil {
			return nil, err
		}
		return handler(ctx, req)
	}

	chainedInterceptor := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		// first apply auth interceptor
		authHandler := grpc_auth.UnaryServerInterceptor(grpcutil.StaticAuthInterceptor(remoteAgentServer.ipcComp.GetAuthToken()))
		return authHandler(ctx, req, info, func(ctx context.Context, req any) (any, error) {
			// then apply session ID interceptor
			return sessionIDInterceptor(ctx, req, info, handler)
		})
	}

	serverOpts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(remoteAgentServer.ipcComp.GetTLSServerConfig())),
		grpc.UnaryInterceptor(chainedInterceptor),
	}

	remoteAgentServer.grpcServer = grpc.NewServer(serverOpts...)

	// Setup lifecycle
	lc.Append(compdef.Hook{
		OnStart: func(_ context.Context) error {
			remoteAgentServer.start()
			return nil
		},
		OnStop: func(_ context.Context) error {
			remoteAgentServer.stop()
			return nil
		},
	})

	return remoteAgentServer, nil
}

// Start the unimplemented remote agent server
func (s *UnimplementedRemoteAgentServer) start() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Get the services from the gRPC server
	for serviceName := range s.grpcServer.GetServiceInfo() {
		s.services = append(s.services, serviceName)
	}

	// Start the gRPC server
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		if err := s.grpcServer.Serve(s.listener); err != nil {
			s.log.Errorf("failed to serve: %v", err)
			// Cancel the context to stop the registration loop
			s.cancel()
			return
		}
	}()

	// Start the registration loop
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		// Create exponential backoff for registration retries
		registrationBackoff := backoff.NewExponentialBackOff()
		registrationBackoff.InitialInterval = 500 * time.Millisecond
		registrationBackoff.MaxInterval = time.Minute
		registrationBackoff.Reset()

		// Start with immediate first registration attempt
		ticker := time.NewTicker(time.Microsecond)
		defer ticker.Stop()

		for {
			select {
			case <-s.ctx.Done():
				s.grpcServer.GracefulStop()
				return
			case <-ticker.C:
				s.sessionIDMutex.RLock()
				sessionID := s.sessionID
				s.sessionIDMutex.RUnlock()

				if sessionID == "" {
					// Registration mode: try to register
					s.log.Debug("Session ID is empty, entering registration loop")
					sessionID, refreshInterval, err := s.registerWithAgent()
					if err != nil {
						// Registration failed, use exponential backoff for next retry
						backoffDuration := registrationBackoff.NextBackOff()
						s.log.Debugf("Registration failed, retrying in %v", backoffDuration)
						ticker.Reset(backoffDuration)
						continue
					}

					// Registration succeeded
					s.sessionIDMutex.Lock()
					s.sessionID = sessionID
					s.sessionIDMutex.Unlock()

					// Reset backoff for next registration cycle and switch to periodic refresh
					registrationBackoff.Reset()
					ticker.Reset(refreshInterval)
				} else {
					// Refresh mode: try to refresh registration
					err := s.refreshRegistration()
					if err != nil {
						s.log.Warnf("failed to refresh registration with Core Agent: %v, entering registration loop", err)
						s.sessionIDMutex.Lock()
						s.sessionID = ""
						s.sessionIDMutex.Unlock()

						// Switch back to exponential backoff for registration
						ticker.Reset(registrationBackoff.NextBackOff())
					}
				}
			}
		}

	}()
}

func (s *UnimplementedRemoteAgentServer) stop() {
	s.cancel()
	s.wg.Wait()
	s.log.Debug("remoteAgentServer stopped")
}

func newAgentSecureClient(ipcComp ipc.Component, agentIpcAddress string) (pbcore.AgentSecureClient, error) {
	conn, err := grpc.NewClient(agentIpcAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(ipcComp.GetTLSClientConfig())),
		grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(ipcComp.GetAuthToken())),
	)
	if err != nil {
		return nil, err
	}

	return pbcore.NewAgentSecureClient(conn), nil
}

// registerWithAgent handles the registration logic with the Core Agent
func (s *UnimplementedRemoteAgentServer) registerWithAgent() (string, time.Duration, error) {
	registerReq := &pbcore.RegisterRemoteAgentRequest{
		Flavor:         s.agentFlavor,
		DisplayName:    s.displayName,
		ApiEndpointUri: s.listener.Addr().String(),
		Services:       s.services,
	}

	s.log.Debugf("Registering with Core Agent at %s...", s.agentIpcAddress)

	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	resp, err := s.agentClient.RegisterRemoteAgent(ctx, registerReq)
	if err != nil {
		s.log.Debugf("failed to register remote agent: %v", err)
		return "", 0, err
	}

	// Store the session ID for use in the session ID interceptor
	s.log.Infof("Registered with Core Agent. Recommended refresh interval of %d seconds.", resp.RecommendedRefreshIntervalSecs)

	// Check that refresh rate is greater than 0 seconds
	var refreshInterval time.Duration
	if resp.RecommendedRefreshIntervalSecs == 0 {
		s.log.Warnf("Recommended refresh interval is 0 seconds, using default refresh interval of %d seconds", s.defaultRefreshInterval)
		refreshInterval = s.defaultRefreshInterval
	} else {
		refreshInterval = time.Duration(resp.RecommendedRefreshIntervalSecs) * time.Second
	}

	return resp.SessionId, refreshInterval, nil
}

// refreshRegistration handles the refresh logic with the Core Agent
func (s *UnimplementedRemoteAgentServer) refreshRegistration() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.queryTimeout)
	defer cancel()

	_, err := s.agentClient.RefreshRemoteAgent(ctx, &pbcore.RefreshRemoteAgentRequest{SessionId: s.sessionID})
	if err != nil {
		return err
	}

	s.log.Debug("Refreshed registration with Core Agent.")
	return nil
}

// GetGRPCServer returns the gRPC server
func (s *UnimplementedRemoteAgentServer) GetGRPCServer() *grpc.Server {
	return s.grpcServer
}

// WaitSessionID blocks until the remote agent is registered and a session ID is available, or ctx is done.
// It returns the session ID or an error if the context is cancelled before registration completes.
func (s *UnimplementedRemoteAgentServer) WaitSessionID(ctx context.Context) (string, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		s.sessionIDMutex.RLock()
		sid := s.sessionID
		s.sessionIDMutex.RUnlock()
		if sid != "" {
			return sid, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}
