// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"fmt"
	"net/http"
	"time"

	gorilla "github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/internal/agent"
	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/observability"
	"github.com/DataDog/datadog-agent/comp/api/grpcserver/helpers"
)

const cmdServerName string = "CMD API Server"
const cmdServerShortName string = "CMD"

func (server *apiServer) startCMDServer(
	cmdAddr string,
	tmf observability.TelemetryMiddlewareFactory,
) (err error) {
	// get the transport we're going to use under HTTP
	server.cmdListener, err = getListener(cmdAddr)
	if err != nil {
		// we use the listener to handle commands for the Agent, there's
		// no way we can recover from this error
		return fmt.Errorf("unable to listen to the given address: %v", err)
	}

	// gRPC server
	grpcServer := server.grpcComponent.BuildServer()

	// Setup multiplexer
	// create the REST HTTP router
	agentMux := gorilla.NewRouter()

	// Validate token for every request
	agentMux.Use(server.ipc.HTTPMiddleware)

	cmdMux := http.NewServeMux()
	cmdMux.Handle(
		"/agent/",
		http.StripPrefix("/agent",
			agent.SetupHandlers(
				agentMux,
				server.endpointProviders,
			)))

	// Add some observability in the API server
	cmdMuxHandler := tmf.Middleware(cmdServerShortName)(cmdMux)
	cmdMuxHandler = observability.LogResponseHandler(cmdServerName)(cmdMuxHandler)

	tlsConfig := server.ipc.GetTLSServerConfig()

	srv := &http.Server{
		Addr:      cmdAddr,
		Handler:   cmdMuxHandler,
		TLSConfig: tlsConfig,
	}

	if grpcServer != nil {
		srv = helpers.NewMuxedGRPCServer(cmdAddr, tlsConfig, grpcServer, cmdMuxHandler, time.Duration(server.cfg.GetInt64("server_timeout"))*time.Second)
	}

	startServer(server.cmdListener, srv, cmdServerName)

	return nil
}
