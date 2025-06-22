// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remoteagentimpl implements the remoteagent component interface
package remoteagentimpl

import (
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	remoteagent "github.com/DataDog/datadog-agent/comp/core/remoteagent/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// Requires defines the dependencies for the remoteagent component
type Requires struct {
	// Remove this field if the component has no lifecycle hooks
	Lifecycle compdef.Lifecycle
	Config    config.Component
	Params    remoteagent.Params
	IPC       ipc.Component
}

// Provides defines the output of the remoteagent component
type Provides struct {
	Comp remoteagent.Component
}

type remoteAgent struct {
	params remoteagent.Params
	client pbcore.AgentSecureClient
	ipc    ipc.Component
}

func newRemoteAgent(params remoteagent.Params, config config.Component, ipc ipc.Component) (*remoteAgent, error) {
	address, err := pkgconfigsetup.GetIPCAddress(config)
	if err != nil {
		return nil, err
	}
	coreIPCAddres := fmt.Sprintf("%v:%v", address, config.GetInt("cmd_port"))

	creds := credentials.NewTLS(ipc.GetTLSClientConfig())

	conn, err := grpc.NewClient(coreIPCAddres,
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(params.AuthToken)),
	)
	if err != nil {
		return nil, err
	}

	return &remoteAgent{
		params: params,
		client: pbcore.NewAgentSecureClient(conn),
		ipc:    ipc,
	}, nil
}

type remoteAgentServer struct {
	started           time.Time
	remoteAgentParams remoteagent.Params
	pbcore.UnimplementedRemoteAgentServer
}

func (s *remoteAgentServer) GetStatusDetails(_ context.Context, _ *pbcore.GetStatusDetailsRequest) (*pbcore.GetStatusDetailsResponse, error) {
	return &pbcore.GetStatusDetailsResponse{
		MainSection: &pbcore.StatusSection{
			Fields: s.remoteAgentParams.StatusCallback(),
		},
		NamedSections: make(map[string]*pbcore.StatusSection),
	}, nil

}

func (s *remoteAgentServer) GetFlareFiles(_ context.Context, _ *pbcore.GetFlareFilesRequest) (*pbcore.GetFlareFilesResponse, error) {
	return &pbcore.GetFlareFilesResponse{
		Files: s.remoteAgentParams.FlareCallback(),
	}, nil
}

func (s *remoteAgentServer) GetTelemetry(_ context.Context, _ *pbcore.GetTelemetryRequest) (*pbcore.GetTelemetryResponse, error) {
	return &pbcore.GetTelemetryResponse{
		Payload: &pbcore.GetTelemetryResponse_PromText{
			PromText: s.remoteAgentParams.TelemetryCallback(),
		},
	}, nil
}

func newRemoteAgentServer(params remoteagent.Params) *remoteAgentServer {
	return &remoteAgentServer{
		started:           time.Now(),
		remoteAgentParams: params,
	}
}

func (r *remoteAgent) buildAndSpawnGrpcServer(server pbcore.RemoteAgentServer) {
	// Make sure we can listen on the intended address.
	listener, err := net.Listen("tcp", r.params.Endpoint)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	serverOpts := []grpc.ServerOption{
		grpc.Creds(credentials.NewTLS(r.ipc.GetTLSServerConfig())),
		grpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(grpcutil.StaticAuthInterceptor(r.params.AuthToken))),
	}

	grpcServer := grpc.NewServer(serverOpts...)
	pbcore.RegisterRemoteAgentServer(grpcServer, server)

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
}

func (r *remoteAgent) start(ctx context.Context) {
	r.buildAndSpawnGrpcServer(newRemoteAgentServer(r.params))
	// TODO find a better initial time
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:

			registrationContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			registerReq := &pbcore.RegisterRemoteAgentRequest{
				Id:          r.params.ID,
				DisplayName: r.params.DisplayName,
				ApiEndpoint: r.params.Endpoint,
				AuthToken:   r.params.AuthToken,
			}

			resp, err := r.client.RegisterRemoteAgent(registrationContext, registerReq)
			if err != nil {
				// TODO
			}
			ticker.Reset(time.Duration(resp.RecommendedRefreshIntervalSecs))
			return
		}
	}
}

func (r *remoteAgent) stop() {
	// TODO
}

// NewComponent creates a new remoteagent component
func NewComponent(reqs Requires) (Provides, error) {
	remoteAgent, err := newRemoteAgent(reqs.Params, reqs.Config, reqs.IPC)

	if err != nil {
		return Provides{}, err
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(ctx context.Context) error {
			remoteAgent.start(ctx)
			return nil
		},

		OnStop: func(context.Context) error {
			remoteAgent.stop()
			return nil
		},
	})

	return Provides{}, nil
}
