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
	"fmt"
	stdLog "log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/cihub/seelog"
	gorilla "github.com/gorilla/mux"
	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/DataDog/datadog-agent/cmd/agent/api/internal/agent"
	"github.com/DataDog/datadog-agent/cmd/agent/api/internal/check"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
	remoteconfig "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo"
)

var listener net.Listener

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

// timeoutHandler limits requests to the enclosed handler to the value
// configured in `server_timeout`.
func timeoutHandlerFunc(otherHandler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline := time.Now().Add(time.Duration(config.Datadog.GetInt64("server_timeout")) * time.Second)

		conn := agent.GetConnection(r)
		_ = conn.SetWriteDeadline(deadline)

		otherHandler.ServeHTTP(w, r)
	})
}

// StartServer creates the router and starts the HTTP server
func StartServer(configService *remoteconfig.Service) error {
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
		grpc.Creds(credentials.NewClientTLSFromCert(tlsCertPool, tlsAddr)),
		grpc.StreamInterceptor(grpc_auth.StreamServerInterceptor(grpcAuth)),
		grpc.UnaryInterceptor(grpc_auth.UnaryServerInterceptor(grpcAuth)),
	}

	s := grpc.NewServer(opts...)
	pb.RegisterAgentServer(s, &server{})
	pb.RegisterAgentSecureServer(s, &serverSecure{configService: configService})

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

	err = pb.RegisterAgentSecureHandlerFromEndpoint(
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

	// apply server_timeout to all handlers in the mux (with a few exceptions
	// such as streaming logs, which reset the deadlines back to zero)
	handler := timeoutHandlerFunc(mux)

	// Use a stack depth of 4 on top of the default one to get a relevant filename in the stdlib
	logWriter, _ := config.NewLogWriter(5, seelog.ErrorLvl)

	srv := &http.Server{
		Addr: tlsAddr,
		// handle grpc calls directly, falling back to `handler` for non-grpc reqs
		Handler: grpcHandlerFunc(s, handler),
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{*tlsKeyPair},
			NextProtos:   []string{"h2"},
			MinVersion:   tls.VersionTLS13,
		},
		ErrorLog: stdLog.New(logWriter, "Error from the agent http API server: ", 0), // log errors to seelog,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			// Store the connection in the context so requests can reference it if needed
			return context.WithValue(ctx, agent.ConnContextKey, c)
		},
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
