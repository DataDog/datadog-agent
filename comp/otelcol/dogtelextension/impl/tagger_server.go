// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogtelextensionimpl

import (
	"context"
	"fmt"
	"net"

	grpcauth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	taggerserver "github.com/DataDog/datadog-agent/comp/core/tagger/server"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// taggerServerWrapper implements minimal pb.AgentSecureServer for tagger only
type taggerServerWrapper struct {
	pb.UnimplementedAgentSecureServer
	taggerSrv *taggerserver.Server
}

// TaggerStreamEntities implements pb.AgentSecureServer
func (w *taggerServerWrapper) TaggerStreamEntities(in *pb.StreamTagsRequest, out pb.AgentSecure_TaggerStreamEntitiesServer) error {
	return w.taggerSrv.TaggerStreamEntities(in, out)
}

// TaggerFetchEntity implements pb.AgentSecureServer
func (w *taggerServerWrapper) TaggerFetchEntity(ctx context.Context, in *pb.FetchEntityRequest) (*pb.FetchEntityResponse, error) {
	return w.taggerSrv.TaggerFetchEntity(ctx, in)
}

// startTaggerServer starts the minimal tagger gRPC server
func (e *dogtelExtension) startTaggerServer() error {
	// 1. Create listener
	addr := fmt.Sprintf("%s:%d", e.config.TaggerServerAddr, e.config.TaggerServerPort)
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to create listener on %s: %w", addr, err)
	}
	e.taggerListener = lis

	// Store the actual port (useful when auto-assigning with port 0)
	e.taggerServerPort = lis.Addr().(*net.TCPAddr).Port

	// 2. Create tagger server component (from comp/core/tagger/server)
	maxEventSize := e.config.TaggerMaxMessageSize / 2
	taggerSrv := taggerserver.NewServer(
		e.tagger,
		e.telemetry,
		maxEventSize,
		e.config.TaggerMaxConcurrentSync,
	)

	// 3. Setup gRPC server with authentication
	var grpcOpts []grpc.ServerOption

	// Get TLS credentials from IPC component
	tlsConf := e.ipc.GetTLSServerConfig()
	if tlsConf != nil {
		creds := credentials.NewTLS(tlsConf)
		grpcOpts = append(grpcOpts, grpc.Creds(creds))
		e.log.Debug("Tagger server: TLS enabled")
	} else {
		e.log.Warn("Tagger server: TLS not configured, running without TLS")
	}

	// Add auth interceptor from IPC
	authInterceptor := grpcauth.UnaryServerInterceptor(grpcutil.StaticAuthInterceptor(e.ipc.GetAuthToken()))
	grpcOpts = append(grpcOpts, grpc.UnaryInterceptor(authInterceptor))

	// Add stream auth interceptor
	streamAuthInterceptor := grpcauth.StreamServerInterceptor(grpcutil.StaticAuthInterceptor(e.ipc.GetAuthToken()))
	grpcOpts = append(grpcOpts, grpc.StreamInterceptor(streamAuthInterceptor))

	// Set max message size
	grpcOpts = append(grpcOpts,
		grpc.MaxRecvMsgSize(e.config.TaggerMaxMessageSize),
		grpc.MaxSendMsgSize(e.config.TaggerMaxMessageSize),
	)

	// 4. Create gRPC server
	e.taggerServer = grpc.NewServer(grpcOpts...)

	// 5. Register tagger service
	pb.RegisterAgentSecureServer(e.taggerServer, &taggerServerWrapper{
		taggerSrv: taggerSrv,
	})

	// 6. Start serving in goroutine
	go func() {
		e.log.Infof("Starting tagger gRPC server on %s (port %d)", addr, e.taggerServerPort)
		if err := e.taggerServer.Serve(lis); err != nil {
			e.log.Errorf("Tagger server error: %v", err)
		}
	}()

	return nil
}

// stopTaggerServer stops the tagger gRPC server gracefully
func (e *dogtelExtension) stopTaggerServer() {
	if e.taggerServer != nil {
		e.log.Info("Stopping tagger gRPC server")
		e.taggerServer.GracefulStop()
		e.taggerServer = nil
	}
	if e.taggerListener != nil {
		e.taggerListener.Close()
		e.taggerListener = nil
	}
}
