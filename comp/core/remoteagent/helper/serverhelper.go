// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package helper implements the helper for the remoteagent component
package helper

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v7"
	"github.com/mdlayher/vsock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/listener"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"

	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/system/socket"
)

// UnimplementedRemoteAgentServer is a wrapper around a gRPC server that implements the RemoteAgentServer protocol.
// It takes care of the registration logic with the Core Agent.
type UnimplementedRemoteAgentServer struct {
	log    log.Component
	config config.Component

	// server infos
	agentFlavor       string
	displayName       string
	services          []string
	registeredAPIURI  string
	cleanupSocketPath string

	// communication components
	ipcComp         ipc.Component
	agentClient     pbcore.RemoteAgentClient
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

	// create the listener at a random port
	ral, err := buildRemoteAgentListener("https://127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	agentClient, err := newRemoteAgentClient(ipcComp, agentIpcAddress, config, log)
	if err != nil {
		log.Errorf("failed to create agent client: %v", err)
		if ral.cleanupSocketPath != "" {
			_ = os.Remove(ral.cleanupSocketPath)
		}
		_ = ral.listener.Close()
		return nil, err
	}

	remoteAgentServer := &UnimplementedRemoteAgentServer{
		ipcComp:                ipcComp,
		log:                    log,
		config:                 config,
		agentIpcAddress:        agentIpcAddress,
		agentFlavor:            agentFlavor,
		displayName:            displayName,
		registeredAPIURI:       ral.apiEndpointURI,
		cleanupSocketPath:      ral.cleanupSocketPath,
		agentClient:            agentClient,
		listener:               ral.listener,
		grpcServer:             nil,
		defaultRefreshInterval: 5 * time.Second,
		queryTimeout:           config.GetDuration("remote_agent.registry.query_timeout"),
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

	// Each impl must call Start() after registering its gRPC services so the
	// service list reported to the core agent is complete and the gRPC server
	// is not accepting RPCs before they are wired.
	lc.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			remoteAgentServer.stop()
			return nil
		},
	})

	return remoteAgentServer, nil
}

// Start begins serving gRPC and starts the RAR registration loop. Impls must
// call this after registering services on GetGRPCServer().
func (s *UnimplementedRemoteAgentServer) Start() {
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
	if s.cleanupSocketPath != "" {
		if err := os.Remove(s.cleanupSocketPath); err != nil && !os.IsNotExist(err) {
			s.log.Warnf("failed to remove remote agent UDS socket %q: %v", s.cleanupSocketPath, err)
		}
	}
	s.log.Debug("remoteAgentServer stopped")
}

// remoteAgentListener bundles the inbound listener for a remote agent with the
// derived metadata needed by the registration loop and shutdown logic.
type remoteAgentListener struct {
	// listener is the net.Listener the gRPC server should Serve on.
	listener net.Listener

	// apiEndpointURI is the value advertised to the Core Agent in the
	// RegisterRemoteAgent RPC (e.g. "https://127.0.0.1:50051" or
	// "unix:///var/run/datadog/remote-agent.sock").
	apiEndpointURI string

	// cleanupSocketPath, when non-empty, is the filesystem path of a UDS that
	// must be removed when the listener is shut down. It is empty for TCP listeners.
	cleanupSocketPath string
}

// buildRemoteAgentListener creates the inbound listener for a remote agent and computes
// the api_endpoint_uri that should be advertised to the Core Agent.
//
// listenURI follows the scheme conventions documented on
// NewUnimplementedRemoteAgentServer. When listenURI is empty, the helper retains its original
// behaviour: a TCP listener on a random localhost port, advertised with the explicit
// "https://" scheme prefix.
func buildRemoteAgentListener(listenURI string) (*remoteAgentListener, error) {
	scheme, rest, hasScheme := strings.Cut(listenURI, "://")
	if !hasScheme {
		return nil, fmt.Errorf("invalid remote agent listen URI %q: missing scheme (expected https://, unix://, or http://)", listenURI)
	}

	switch strings.ToLower(scheme) {
	case "unix":
		socketPath, err := unixSocketPath(rest)
		if err != nil {
			return nil, err
		}
		if err := removeStaleSocket(socketPath); err != nil {
			return nil, err
		}
		l, err := net.Listen("unix", socketPath)
		if err != nil {
			return nil, fmt.Errorf("failed to listen on UDS %q: %w", socketPath, err)
		}
		if err := os.Chmod(socketPath, 0700); err != nil {
			_ = l.Close()
			_ = os.Remove(socketPath)
			return nil, fmt.Errorf("failed to restrict permissions on UDS %q: %w", socketPath, err)
		}
		return &remoteAgentListener{listener: l, apiEndpointURI: listenURI, cleanupSocketPath: socketPath}, nil
	case "https":
		l, err := listener.GetListener(rest)
		if err != nil {
			return nil, err
		}
		return &remoteAgentListener{listener: l, apiEndpointURI: "https://" + l.Addr().String()}, nil
	case "http":
		// http:// is permitted by the protocol but the helper itself always serves TLS,
		// so we refuse to set up a server that advertises plaintext to the registry.
		return nil, errors.New("http:// scheme is not supported on the remote agent server side (use https:// or unix://)")
	default:
		return nil, fmt.Errorf("unsupported remote agent listen URI scheme %q", scheme)
	}
}

// unixSocketPath extracts the filesystem path from the rest of a "unix://" URI.
// Both "unix:///abs/path" (rest = "/abs/path") and the legacy "unix://abs/path" forms
// are accepted; relative paths are rejected since gRPC's unix resolver requires absolute paths.
func unixSocketPath(rest string) (string, error) {
	if rest == "" {
		return "", errors.New("invalid unix:// URI: empty socket path")
	}
	// gRPC's unix scheme supports both unix:///abs and unix:/abs forms; net.Listen needs the plain path.
	path := rest
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path, nil
}

func removeStaleSocket(socketPath string) error {
	info, err := os.Stat(socketPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to stat existing socket path %q: %w", socketPath, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("refusing to bind UDS at %q: path exists and is not a socket", socketPath)
	}
	if err := os.Remove(socketPath); err != nil {
		return fmt.Errorf("failed to remove stale UDS at %q: %w", socketPath, err)
	}
	return nil
}

func newRemoteAgentClient(ipcComp ipc.Component, agentIpcAddress string, cfg config.Component, log log.Component) (pbcore.RemoteAgentClient, error) {
	conn, err := dialCoreAgent(agentIpcAddress, ipcComp.GetAuthToken(), ipcComp.GetTLSClientConfig(), cfg.GetString("vsock_addr"), log)
	if err != nil {
		return nil, err
	}
	return pbcore.NewRemoteAgentClient(conn), nil
}

// NewAgentSecureClient dials the core agent for pre-FX consumers that don't yet
// have an ipc.Component / config.Component. The returned ClientConn is owned by
// the caller. vsockAddr non-empty switches to vsock.
func NewAgentSecureClient(agentIpcAddress, authToken string, tlsConfig *tls.Config, vsockAddr string, log log.Component) (pbcore.AgentSecureClient, *grpc.ClientConn, error) {
	conn, err := dialCoreAgent(agentIpcAddress, authToken, tlsConfig, vsockAddr, log)
	if err != nil {
		return nil, nil, err
	}
	return pbcore.NewAgentSecureClient(conn), conn, nil
}

func dialCoreAgent(agentIpcAddress, authToken string, tlsConfig *tls.Config, vsockAddr string, log log.Component) (*grpc.ClientConn, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
		grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(authToken)),
	}

	if vsockAddr != "" {
		cid, err := socket.ParseVSockAddress(vsockAddr)
		if err != nil {
			return nil, err
		}

		_, sPort, err := net.SplitHostPort(agentIpcAddress)
		if err != nil {
			return nil, err
		}

		cmdPort, parseErr := strconv.ParseUint(sPort, 10, 16)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid vsock socket path '%s'", agentIpcAddress)
		}

		if cmdPort == 0 {
			return nil, errors.New("invalid port '0' for vsock")
		}

		opts = append(opts, grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			log.Debugf("dialing vsock address with CID %d and port %d", cid, cmdPort)
			return vsock.Dial(cid, uint32(cmdPort), &vsock.Config{})
		}))
	}

	return grpc.NewClient(agentIpcAddress, opts...)
}

// RegistrationRequest is the input for RegisterRemoteAgent.
type RegistrationRequest struct {
	Flavor         string
	DisplayName    string
	APIEndpointURI string
	Services       []string
}

// RegisterRemoteAgent calls RegisterRemoteAgent on the core agent. If the
// core's recommended refresh interval is 0, defaultRefreshInterval is used.
func RegisterRemoteAgent(ctx context.Context, client pbcore.AgentSecureClient, req RegistrationRequest, queryTimeout, defaultRefreshInterval time.Duration, log log.Component) (string, time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, queryTimeout)
	defer cancel()

	resp, err := client.RegisterRemoteAgent(ctx, &pbcore.RegisterRemoteAgentRequest{
		Flavor:         req.Flavor,
		DisplayName:    req.DisplayName,
		ApiEndpointUri: req.APIEndpointURI,
		Services:       req.Services,
	})
	if err != nil {
		log.Debugf("failed to register remote agent: %v", err)
		return "", 0, err
	}

	log.Infof("Registered with Remote Agent Registry (session_id=%s). Recommended refresh interval: %d seconds.", resp.SessionId, resp.RecommendedRefreshIntervalSecs)

	refreshInterval := time.Duration(resp.RecommendedRefreshIntervalSecs) * time.Second
	if resp.RecommendedRefreshIntervalSecs == 0 {
		log.Warnf("Recommended refresh interval is 0 seconds, using default refresh interval of %s", defaultRefreshInterval)
		refreshInterval = defaultRefreshInterval
	}
	return resp.SessionId, refreshInterval, nil
}

// registerWithAgent handles the registration logic with the Core Agent
func (s *UnimplementedRemoteAgentServer) registerWithAgent() (string, time.Duration, error) {
	registerReq := &pbcore.RegisterRemoteAgentRequest{
		Flavor:         s.agentFlavor,
		DisplayName:    s.displayName,
		ApiEndpointUri: s.registeredAPIURI,
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

	// Store the session ID for use in the session ID interceptor and config streaming
	s.log.Infof("Registered with Remote Agent Registry for config streaming (session_id=%s). Recommended refresh interval: %d seconds.", resp.SessionId, resp.RecommendedRefreshIntervalSecs)

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
