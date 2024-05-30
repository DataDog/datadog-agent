// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package impl

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

	"github.com/DataDog/datadog-agent/comp/api/api"
	"github.com/DataDog/datadog-agent/comp/api/api/impl/internal/agent"
	"github.com/DataDog/datadog-agent/comp/api/api/impl/internal/check"
	apiutils "github.com/DataDog/datadog-agent/comp/api/api/impl/utils"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/status"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	taggerserver "github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl/server"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	workloadmetaServer "github.com/DataDog/datadog-agent/comp/core/workloadmeta/server"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	replay "github.com/DataDog/datadog-agent/comp/dogstatsd/replay/def"
	dogstatsdServer "github.com/DataDog/datadog-agent/comp/dogstatsd/server"
	logsAgent "github.com/DataDog/datadog-agent/comp/logs/agent"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservice"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcservicemrf"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/config"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

const cmdServerName string = "CMD API Server"

var cmdListener net.Listener

func startCMDServer(
	cmdAddr string,
	tlsConfig *tls.Config,
	tlsCertPool *x509.CertPool,
	configService optional.Option[rcservice.Component],
	configServiceMRF optional.Option[rcservicemrf.Component],
	dogstatsdServer dogstatsdServer.Component,
	capture replay.Component,
	pidMap pidmap.Component,
	wmeta workloadmeta.Component,
	taggerComp tagger.Component,
	logsAgent optional.Option[logsAgent.Component],
	senderManager sender.DiagnoseSenderManager,
	secretResolver secrets.Component,
	statusComponent status.Component,
	collector optional.Option[collector.Component],
	ac autodiscovery.Component,
	providers []api.EndpointProvider,
) (err error) {
	// get the transport we're going to use under HTTP
	cmdListener, err = getListener(cmdAddr)
	if err != nil {
		// we use the listener to handle commands for the Agent, there's
		// no way we can recover from this error
		return fmt.Errorf("unable to listen to the given address: %v", err)
	}

	// gRPC server
	authInterceptor := grpcutil.AuthInterceptor(parseToken)
	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewClientTLSFromCert(tlsCertPool, cmdAddr)),
		grpc.StreamInterceptor(grpc_auth.StreamServerInterceptor(authInterceptor)),
		grpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(authInterceptor)),
	}

	s := grpc.NewServer(opts...)
	pb.RegisterAgentServer(s, &server{})
	pb.RegisterAgentSecureServer(s, &serverSecure{
		configService:    configService,
		configServiceMRF: configServiceMRF,
		taggerServer:     taggerserver.NewServer(taggerComp),
		// TODO(components): decide if workloadmetaServer should be componentized itself
		workloadmetaServer: workloadmetaServer.NewServer(wmeta),
		dogstatsdServer:    dogstatsdServer,
		capture:            capture,
		pidMap:             pidMap,
	})

	dcreds := credentials.NewTLS(&tls.Config{
		ServerName: cmdAddr,
		RootCAs:    tlsCertPool,
	})
	dopts := []grpc.DialOption{grpc.WithTransportCredentials(dcreds)}

	// starting grpc gateway
	ctx := context.Background()
	gwmux := runtime.NewServeMux()
	err = pb.RegisterAgentHandlerFromEndpoint(
		ctx, gwmux, cmdAddr, dopts)
	if err != nil {
		return fmt.Errorf("error registering agent handler from endpoint %s: %v", cmdAddr, err)
	}

	err = pb.RegisterAgentSecureHandlerFromEndpoint(
		ctx, gwmux, cmdAddr, dopts)
	if err != nil {
		return fmt.Errorf("error registering agent secure handler from endpoint %s: %v", cmdAddr, err)
	}

	// Setup multiplexer
	// create the REST HTTP router
	agentMux := gorilla.NewRouter()
	checkMux := gorilla.NewRouter()

	// Validate token for every request
	agentMux.Use(validateToken)
	checkMux.Use(validateToken)

	cmdMux := http.NewServeMux()
	cmdMux.Handle(
		"/agent/",
		http.StripPrefix("/agent",
			agent.SetupHandlers(
				agentMux,
				wmeta,
				logsAgent,
				senderManager,
				secretResolver,
				statusComponent,
				collector,
				ac,
				providers,
			)))
	cmdMux.Handle("/check/", http.StripPrefix("/check", check.SetupHandlers(checkMux)))
	cmdMux.Handle("/", gwmux)

	// Add some observability in the API server
	cmdMuxHandler := apiutils.LogResponseHandler(cmdServerName)(cmdMux)

	srv := grpcutil.NewMuxedGRPCServer(
		cmdAddr,
		tlsConfig,
		s,
		grpcutil.TimeoutHandlerFunc(cmdMuxHandler, time.Duration(config.Datadog().GetInt64("server_timeout"))*time.Second),
	)

	startServer(cmdListener, srv, cmdServerName)

	return nil
}

// ServerAddress returns the server address.
func ServerAddress() *net.TCPAddr {
	return cmdListener.Addr().(*net.TCPAddr)
}

func stopCMDServer() {
	stopServer(cmdListener, cmdServerName)
}
