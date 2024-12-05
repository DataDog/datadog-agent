// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"context"
	"errors"
	"fmt"

	grpc_auth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	grpccontext "github.com/DataDog/datadog-agent/pkg/util/grpc/context"
)

// verifierFunc receives the token passed in the request headers, and returns
// arbitrary information about the token to be stored in the request context,
// or an error if the token is not valid.
type verifierFunc func(string) (interface{}, error)

// AuthInterceptor is a gRPC interceptor that extracts an auth token from the
// request headers, and validates it using the provided func.
func AuthInterceptor(verifier verifierFunc) grpc_auth.AuthFunc {
	return func(ctx context.Context) (context.Context, error) {
		token, err := grpc_auth.AuthFromMD(ctx, "Bearer")
		if err != nil {
			return nil, err
		}

		tokenInfo, err := verifier(token)
		if err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "invalid auth token: %v", err)
		}

		return context.WithValue(ctx, grpccontext.ContextKeyTokenInfoID, tokenInfo), nil
	}
}

// StaticAuthInterceptor is a gRPC interceptor that extracts an auth token from the request headers, and validates it
// using the given token.
func StaticAuthInterceptor(token string) grpc_auth.AuthFunc {
	return AuthInterceptor(func(reqToken string) (interface{}, error) {
		if reqToken != token {
			return struct{}{}, errors.New("invalid session token")
		}

		return struct{}{}, nil
	})
}

type bearerTokenAuth struct {
	token string
}

func (b bearerTokenAuth) GetRequestMetadata(_ context.Context, _ ...string) (map[string]string, error) {
	return map[string]string{
		"authorization": fmt.Sprintf("Bearer %s", b.token),
	}, nil
}

func (b bearerTokenAuth) RequireTransportSecurity() bool {
	return true
}

// NewBearerTokenAuth creates a set of per-RPC credentials that uses a bearer token for authentication/authorization.
//
// This credentials implementation requires the connection to be secure (i.e. using TLS).
func NewBearerTokenAuth(token string) credentials.PerRPCCredentials {
	return bearerTokenAuth{token: token}
}
