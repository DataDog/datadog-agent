// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"errors"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	grpcutil "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// requiresAuth reports whether the endpoints of the given socket family must be authenticated.
//
// A TCP endpoint can be reached by any local user, so it requires TLS and the IPC auth token.
// Unix sockets are protected by their file permissions, and vsock endpoints are deliberately left
// unauthenticated so that the auth token never has to be shared with a micro VM.
func requiresAuth(family string) bool {
	return family == "tcp"
}

// serverOptions returns the gRPC server options for an endpoint of the given socket family
func serverOptions(family string, ipcComp ipc.Component) ([]grpc.ServerOption, error) {
	if !requiresAuth(family) {
		return grpcutil.ServerOptionsWithMetrics(), nil
	}

	if ipcComp == nil {
		return nil, errors.New("the IPC component is required to serve runtime security gRPC over TCP")
	}

	authInterceptor := grpcutil.StaticAuthInterceptor(ipcComp.GetAuthToken())
	return grpcutil.ServerOptionsWithMetricsAndAuth(
		grpc_auth.UnaryServerInterceptor(authInterceptor),
		grpc_auth.StreamServerInterceptor(authInterceptor),
		grpc.Creds(credentials.NewTLS(ipcComp.GetTLSServerConfig())),
	), nil
}

// ClientOptions returns the gRPC dial options to connect to an endpoint of the given socket family.
//
// For TCP endpoints, the connection uses the IPC TLS configuration to authenticate the server, and
// sends the IPC auth token with every call.
func ClientOptions(family string, ipcComp ipc.Component) ([]grpc.DialOption, error) {
	if !requiresAuth(family) {
		return []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}, nil
	}

	if ipcComp == nil {
		return nil, errors.New("the IPC component is required to connect to runtime security gRPC over TCP")
	}

	return []grpc.DialOption{
		grpc.WithTransportCredentials(credentials.NewTLS(ipcComp.GetTLSClientConfig())),
		grpc.WithPerRPCCredentials(grpcutil.NewBearerTokenAuth(ipcComp.GetAuthToken())),
	}, nil
}
