// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.
*/
package api

import (
	"context"
	"crypto/tls"
	"fmt"
	stdLog "log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"github.com/StackVista/stackstate-agent/cmd/agent/api/agent"
	"github.com/StackVista/stackstate-agent/cmd/agent/api/check"
        pb "github.com/StackVista/stackstate-agent/cmd/agent/api/pb"
	"github.com/StackVista/stackstate-agent/pkg/api/util"
	"github.com/StackVista/stackstate-agent/pkg/config"
	hostutil "github.com/StackVista/stackstate-agent/pkg/util"
	gorilla "github.com/gorilla/mux"
)

var (
	listener net.Listener
)

type server struct {
	pb.UnimplementedAgentServer
}

func (s *server) GetHostname(ctx context.Context, in *pb.HostnameRequest) (*pb.HostnameReply, error) {
	h, err := hostutil.GetHostname()
	if err != nil {
		return &pb.HostnameReply{}, err
	}
	return &pb.HostnameReply{Hostname: h}, nil
}

// grpcHandlerFunc returns an http.Handler that delegates to grpcServer on incoming gRPC
// connections or otherHandler otherwise. Copied from cockroachdb.
func grpcHandlerFunc(grpcServer *grpc.Server, otherHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// This is a partial recreation of gRPC's internal checks https://github.com/grpc/grpc-go/pull/514/files#diff-95e9a25b738459a2d3030e1e6fa2a718R61
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			otherHandler.ServeHTTP(w, r)
		}
	})
}

// StartServer creates the router and starts the HTTP server
func StartServer() error {

	initializeTLS()

	// get the transport we're going to use under HTTP
	var err error
	listener, err = getListener()
	if err != nil {
		// we use the listener to handle commands for the Agent, there's
		// no way we can recover from this error
		return fmt.Errorf("Unable to create the api server: %v", err)
	}

	err = util.CreateAndSetAuthToken()
	if err != nil {
		return err
	}

	// gRPC server
	mux := http.NewServeMux()
	opts := []grpc.ServerOption{
		grpc.Creds(credentials.NewClientTLSFromCert(tlsCertPool, tlsAddr))}

	s := grpc.NewServer(opts...)
	pb.RegisterAgentServer(s, &server{})

	dcreds := credentials.NewTLS(&tls.Config{
		ServerName: tlsAddr,
		RootCAs:    tlsCertPool,
	})
	dopts := []grpc.DialOption{grpc.WithTransportCredentials(dcreds)}

	// starting grpc gateway
	ctx := context.Background()
	gwmux := runtime.NewServeMux()
	err = pb.RegisterAgentHandlerFromEndpoint(
		ctx, gwmux, tlsAddr, dopts)
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

	mux.Handle("/agent/", http.StripPrefix("/agent", agent.SetupHandlers(agentMux)))
	mux.Handle("/check/", http.StripPrefix("/check", check.SetupHandlers(checkMux)))
	mux.Handle("/", gwmux)

	srv := &http.Server{
		Addr:    tlsAddr,
		Handler: grpcHandlerFunc(s, mux),
		// Handler: grpcHandlerFunc(s, r),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{*tlsKeyPair},
			NextProtos:   []string{"h2"},
		},
		ErrorLog: stdLog.New(&config.ErrorLogWriter{
			AdditionalDepth: 4, // Use a stack depth of 4 on top of the default one to get a relevant filename in the stdlib
		}, "Error from the agent http API server: ", 0), // log errors to seelog,
		WriteTimeout: config.Datadog.GetDuration("server_timeout") * time.Second,
	}

	tlsListener := tls.NewListener(listener, srv.TLSConfig)

	go srv.Serve(tlsListener) //nolint:errcheck
	return nil
}

// StopServer closes the connection and the server
// stops listening to new commands.
func StopServer() {
	if listener != nil {
		listener.Close()
	}
}

// ServerAddress retruns the server address.
func ServerAddress() *net.TCPAddr {
	return listener.Addr().(*net.TCPAddr)
}

func validateToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := util.Validate(w, r); err != nil {
			return
		}
		next.ServeHTTP(w, r)
	})
}
