// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// Default values for Remote Agent Registry registration
	defaultAgentID             = "system-probe"
	defaultDisplayName         = "System Probe"
	defaultRefreshInterval     = 60 * time.Second
	defaultRegistrationTimeout = 5 * time.Second
)

// RemoteAgentRegistryClient manages registration with the Remote Agent Registry
type RemoteAgentRegistryClient struct {
	agentID       string
	displayName   string
	grpcEndpoint  string
	authToken     string
	coreAgentAddr string
	client        pbcore.AgentSecureClient
	ipcComp       ipc.Component
	stopCh        chan struct{}
}

// NewRemoteAgentRegistryClient creates a new Remote Agent Registry client for system-probe
func NewRemoteAgentRegistryClient(grpcEndpoint, authToken string, config model.Reader, ipcComp ipc.Component) (*RemoteAgentRegistryClient, error) {
	// Get core agent IPC address (following internal/remote-agent pattern)
	coreAgentIPC, err := pkgconfigsetup.GetIPCAddress(config)
	if err != nil {
		return nil, fmt.Errorf("failed to get core agent IPC address: %w", err)
	}

	// Core agent gRPC server runs on port 5003 (secure) by default
	// GetIPCAddress returns just the host, we need to add the gRPC port
	coreAgentAddr := coreAgentIPC + ":5003"

	// Create secure gRPC client to core agent using IPC component
	client, err := newAgentSecureClient(coreAgentAddr, ipcComp)
	if err != nil {
		return nil, fmt.Errorf("failed to create core agent client: %w", err)
	}

	return &RemoteAgentRegistryClient{
		agentID:       defaultAgentID,
		displayName:   defaultDisplayName,
		grpcEndpoint:  grpcEndpoint,
		authToken:     authToken,
		coreAgentAddr: coreAgentAddr,
		client:        client,
		ipcComp:       ipcComp,
		stopCh:        make(chan struct{}),
	}, nil
}

// Register registers system-probe with the core agent's Remote Agent Registry and starts periodic refresh
func (r *RemoteAgentRegistryClient) Register() error {
	log.Infof("Registering system-probe with Core Agent Remote Agent Registry at %s...", r.coreAgentAddr)

	// Perform initial registration
	resp, err := r.registerWithRetry()
	if err != nil {
		return fmt.Errorf("failed to register with core agent: %w", err)
	}

	refreshInterval := time.Duration(resp.RecommendedRefreshIntervalSecs) * time.Second
	if refreshInterval == 0 {
		refreshInterval = defaultRefreshInterval
	}

	log.Infof("Registered with Core Agent Remote Agent Registry. Refresh interval: %v", refreshInterval)

	// Start periodic refresh in background
	go r.periodicRefresh(refreshInterval)

	return nil
}

// Stop stops the Remote Agent Registry client and periodic refresh
func (r *RemoteAgentRegistryClient) Stop() {
	close(r.stopCh)
}

// registerWithRetry performs the actual registration with retry logic
func (r *RemoteAgentRegistryClient) registerWithRetry() (*pbcore.RegisterRemoteAgentResponse, error) {
	registerReq := &pbcore.RegisterRemoteAgentRequest{
		Id:          r.agentID,
		DisplayName: r.displayName,
		ApiEndpoint: r.grpcEndpoint,
		AuthToken:   r.authToken,
	}

	// Create context with Bearer token authentication (production pattern)
	agentAuthToken := r.ipcComp.GetAuthToken()
	ctx := metadata.NewOutgoingContext(
		context.Background(),
		metadata.MD{
			"authorization": []string{fmt.Sprintf("Bearer %s", agentAuthToken)},
		},
	)

	ctx, cancel := context.WithTimeout(ctx, defaultRegistrationTimeout)
	defer cancel()

	resp, err := r.client.RegisterRemoteAgent(ctx, registerReq)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// periodicRefresh handles periodic re-registration with the core agent
func (r *RemoteAgentRegistryClient) periodicRefresh(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Create context with Bearer token authentication (same as initial registration)
			agentAuthToken := r.ipcComp.GetAuthToken()
			ctx := metadata.NewOutgoingContext(
				context.Background(),
				metadata.MD{
					"authorization": []string{fmt.Sprintf("Bearer %s", agentAuthToken)},
				},
			)
			ctx, cancel := context.WithTimeout(ctx, defaultRegistrationTimeout)
			_, err := r.client.RegisterRemoteAgent(ctx, &pbcore.RegisterRemoteAgentRequest{
				Id:          r.agentID,
				DisplayName: r.displayName,
				ApiEndpoint: r.grpcEndpoint,
				AuthToken:   r.authToken,
			})
			cancel()

			if err != nil {
				log.Errorf("Failed to refresh Remote Agent Registry registration: %v", err)
			} else {
				log.Debugf("Refreshed Remote Agent Registry registration")
			}
		case <-r.stopCh:
			log.Info("Stopping Remote Agent Registry client periodic refresh")
			return
		}
	}
}

// newAgentSecureClient creates a secure gRPC client to the core agent
// Following the pattern from remote tagger and other processes
func newAgentSecureClient(agentIpcAddress string, ipcComp ipc.Component) (pbcore.AgentSecureClient, error) {
	// Get auth token and TLS config from IPC component
	tlsConfig := ipcComp.GetTLSClientConfig()
	if tlsConfig == nil {
		return nil, fmt.Errorf("no TLS config available from IPC component")
	}

	// Create gRPC connection with TLS from IPC component
	conn, err := grpc.NewClient(
		agentIpcAddress,
		grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create gRPC connection: %w", err)
	}

	return pbcore.NewAgentSecureClient(conn), nil
}
