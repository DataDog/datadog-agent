// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"net/http"
	"time"

	gorilla "github.com/gorilla/mux"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/DataDog/datadog-agent/cmd/agent/api/internal/agent"
	"github.com/DataDog/datadog-agent/cmd/agent/api/internal/check"
	"github.com/DataDog/datadog-agent/comp/core/flare"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	workloadmetaServer "github.com/DataDog/datadog-agent/comp/core/workloadmeta/server"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/replay"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	dogstatsddebug "github.com/DataDog/datadog-agent/comp/dogstatsd/serverDebug"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/metadata/host"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryagent"
	"github.com/DataDog/datadog-agent/comp/metadata/inventoryhost"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/config"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	taggerserver "github.com/DataDog/datadog-agent/pkg/tagger/server"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

var apiListener net.Listener

func startAPIServer(
	apiAddr string,
	tlsConfig *tls.Config,
	tlsCertPool *x509.CertPool,
	configService *remoteconfig.Service,
	flare flare.Component,
	dogstatsdServer dogstatsdServer.Component,
	capture replay.Component,
	serverDebug dogstatsddebug.Component,
	wmeta workloadmeta.Component,
	logsAgent optional.Option[logsAgent.Component],
	senderManager sender.DiagnoseSenderManager,
	hostMetadata host.Component,
	invAgent inventoryagent.Component,
	invHost inventoryhost.Component,
) (err error) {
	// get the transport we're going to use under HTTP
	apiListener, err = getListener(apiAddr)
	if err != nil {
		// we use the listener to handle commands for the Agent, there's
		// no way we can recover from this error
		return fmt.Errorf("Unable to create the api server: %v", err)
	}

	// gRPC server
	authInterceptor := grpcutil.AuthInterceptor(parseToken)
	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewClientTLSFromCert(tlsCertPool, apiAddr)),
		grpc.StreamInterceptor(grpc_auth.StreamServerInterceptor(authInterceptor)),
		grpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(authInterceptor)),
	}

	s := grpc.NewServer(opts...)
	pb.RegisterAgentServer(s, &server{})
	pb.RegisterAgentSecureServer(s, &serverSecure{
		configService: configService,
		taggerServer:  taggerserver.NewServer(tagger.GetDefaultTagger()),
		// TODO(components): decide if workloadmetaServer should be componentized itself
		workloadmetaServer: workloadmetaServer.NewServer(wmeta),
		dogstatsdServer:    dogstatsdServer,
		capture:            capture,
	})

	dcreds := credentials.NewTLS(&tls.Config{
		ServerName: apiAddr,
		RootCAs:    tlsCertPool,
	})
	dopts := []grpc.DialOption{grpc.WithTransportCredentials(dcreds)}

	// starting grpc gateway
	ctx := context.Background()
	gwmux := runtime.NewServeMux()
	err = pb.RegisterAgentHandlerFromEndpoint(
		ctx, gwmux, apiAddr, dopts)
	if err != nil {
		return fmt.Errorf("error registering agent handler from endpoint %s: %v", apiAddr, err)
	}

	err = pb.RegisterAgentSecureHandlerFromEndpoint(
		ctx, gwmux, apiAddr, dopts)
	if err != nil {
		return fmt.Errorf("error registering agent secure handler from endpoint %s: %v", apiAddr, err)
	}

	// Setup multiplexer
	// create the REST HTTP router
	agentMux := gorilla.NewRouter()
	checkMux := gorilla.NewRouter()
	// Validate token for every request
	agentMux.Use(validateToken)
	checkMux.Use(validateToken)

	apiMux := http.NewServeMux()
	apiMux.Handle(
		"/agent/",
		http.StripPrefix("/agent",
			agent.SetupHandlers(
				agentMux,
				flare,
				dogstatsdServer,
				serverDebug,
				wmeta,
				logsAgent,
				senderManager,
				hostMetadata,
				invAgent,
				invHost,
			)))
	apiMux.Handle("/check/", http.StripPrefix("/check", check.SetupHandlers(checkMux)))
	apiMux.Handle("/", gwmux)

	srv := grpcutil.NewMuxedGRPCServer(
		apiAddr,
		tlsConfig,
		s,
		grpcutil.TimeoutHandlerFunc(apiMux, time.Duration(config.Datadog.GetInt64("server_timeout"))*time.Second),
	)

	startServer(apiListener, srv, "API server")

	return nil
}

// ServerAddress returns the server address.
func ServerAddress() *net.TCPAddr {
	return apiListener.Addr().(*net.TCPAddr)
}
