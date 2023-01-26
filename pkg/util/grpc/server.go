// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"time"

	"google.golang.org/grpc"
)

type contextKey struct {
	key string
}

var ConnContextKey = &contextKey{"http-connection"}

// NewMuxedGRPCServer returns an http.Server that multiplexes connections
// between a gRPC server and an HTTP handler.
func NewMuxedGRPCServer(addr string, tlsConfig *tls.Config, grpcServer *grpc.Server, httpHandler http.Handler) *http.Server {
	// our gRPC clients do not handle protocol negotiation, so we need to force
	// HTTP/2
	tlsConfig.NextProtos = []string{"h2"}

	return &http.Server{
		Addr:      addr,
		Handler:   handlerWithFallback(grpcServer, httpHandler),
		TLSConfig: tlsConfig,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			// Store the connection in the context so requests can reference it if needed
			return context.WithValue(ctx, ConnContextKey, c)
		},
	}
}

// TimeoutHandlerFunc returns an HTTP handler that times out after a duration.
// This is useful for muxed gRPC servers where http.Server cannot have a
// timeout when handling streaming, long running connections.
func TimeoutHandlerFunc(httpHandler http.Handler, timeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline := time.Now().Add(timeout)

		conn := r.Context().Value(ConnContextKey).(net.Conn)
		_ = conn.SetWriteDeadline(deadline)

		httpHandler.ServeHTTP(w, r)
	})
}

// handlerWithFallback returns an http.Handler that delegates to grpcServer on
// incoming gRPC connections or httpServer otherwise. Copied from
// cockroachdb.
func handlerWithFallback(grpcServer *grpc.Server, httpServer http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This is a partial recreation of gRPC's internal checks
		// https://github.com/grpc/grpc-go/pull/514/files#diff-95e9a25b738459a2d3030e1e6fa2a718R61
		if r.ProtoMajor == 2 && strings.Contains(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			httpServer.ServeHTTP(w, r)
		}
	})
}
