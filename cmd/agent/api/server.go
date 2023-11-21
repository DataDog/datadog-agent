// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.
*/
package api

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	stdLog "log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/cihub/seelog"
	"github.com/gorilla/mux"
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
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/tagger"
	taggerserver "github.com/DataDog/datadog-agent/pkg/tagger/server"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

var apiListener net.Listener
var apiConfigListener net.Listener

func startServer(listener net.Listener, srv *http.Server, name string) {
	// Use a stack depth of 4 on top of the default one to get a relevant filename in the stdlib
	logWriter, _ := config.NewLogWriter(5, seelog.ErrorLvl)

	srv.ErrorLog = stdLog.New(logWriter, fmt.Sprintf("Error from the agent http %s: ", name), 0) // log errors to seelog

	tlsListener := tls.NewListener(listener, srv.TLSConfig)

	go srv.Serve(tlsListener) //nolint:errcheck

	log.Infof("%s started on %s", name, listener.Addr().String())
}

// StartServer creates the router and starts the HTTP server
func StartServer(
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
) error {
	apiAddr, err := getIPCAddressPort()
	if err != nil {
		panic("unable to get IPC address and port")
	}

	extraHosts := []string{apiAddr}
	ipcHost, err := config.GetIPCAddress()
	if err != nil {
		return err
	}
	apiConfigPort := config.Datadog.GetInt("api_config_port")
	apiConfigEnabled := apiConfigPort > 0
	apiConfigHostPort := net.JoinHostPort(ipcHost, strconv.Itoa(apiConfigPort))

	if apiConfigEnabled {
		extraHosts = append(extraHosts, apiConfigHostPort)
	}

	tlsKeyPair, tlsCertPool := initializeTLS(extraHosts...)

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{*tlsKeyPair},
		NextProtos:   []string{"h2"},
		MinVersion:   tls.VersionTLS12,
	}

	if err := util.CreateAndSetAuthToken(); err != nil {
		return err
	}

	if err := startAPIServer(
		apiAddr, tlsConfig, tlsCertPool,
		configService, flare, dogstatsdServer,
		capture, serverDebug, wmeta, logsAgent,
		senderManager, hostMetadata, invAgent,
	); err != nil {
		return err
	}

	if apiConfigEnabled {
		if err := startConfigServer(apiConfigHostPort, tlsConfig); err != nil {
			StopServer()
			return err
		}
	}

	return nil
}

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
		panic(err)
	}

	err = pb.RegisterAgentSecureHandlerFromEndpoint(
		ctx, gwmux, apiAddr, dopts)
	if err != nil {
		panic(err)
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

func startConfigServer(apiConfigHostPort string, tlsConfig *tls.Config) (err error) {
	apiConfigListener, err = getListener(apiConfigHostPort)
	if err != nil {
		return err
	}

	configEndpointHandler := func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)

		body, err := getConfigMarshalled(vars["path"])
		if err != nil {
			body, _ := json.Marshal(map[string]string{"error": err.Error()})
			http.Error(w, string(body), http.StatusBadRequest)
			return
		}
		_, _ = w.Write(body)
	}

	configEndpointMux := gorilla.NewRouter()
	configEndpointMux.HandleFunc("/", configEndpointHandler).Methods("GET")
	configEndpointMux.HandleFunc("/.", configEndpointHandler).Methods("GET")
	configEndpointMux.HandleFunc("/{path}", configEndpointHandler).Methods("GET")
	configEndpointMux.Use(validateToken)

	apiConfigMux := http.NewServeMux()
	apiConfigMux.Handle(
		"/config/",
		http.StripPrefix("/config", configEndpointMux))

	apiConfigServer := &http.Server{
		Addr:      apiConfigHostPort,
		Handler:   http.TimeoutHandler(apiConfigMux, time.Duration(config.Datadog.GetInt64("server_timeout"))*time.Second, "timeout"),
		TLSConfig: tlsConfig,
	}

	startServer(apiConfigListener, apiConfigServer, "Config API server")

	return nil
}

func getConfigMarshalled(path string) ([]byte, error) {
	if path == "." {
		path = ""
	}

	var data interface{}
	if path == "" {
		data = config.Datadog.AllSettings()
	} else {
		data = config.Datadog.Get(path)
	}

	if data == nil {
		return nil, fmt.Errorf("no runtime setting found for %s", path)
	}

	return json.Marshal(map[string]interface{}{
		"request": path,
		"value":   data,
	})
}

// StopServer closes the connection and the server
// stops listening to new commands.
func StopServer() {
	if apiListener != nil {
		apiListener.Close()
	}
	if apiConfigListener != nil {
		apiConfigListener.Close()
	}
}

// ServerAddress returns the server address.
func ServerAddress() *net.TCPAddr {
	return apiListener.Addr().(*net.TCPAddr)
}
