// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package grpc provides utility functions for working with gRPC.
package grpc

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// RequireClientCert creates a gRPC unary interceptor that ensures clients provide a certificate.
// The certificate's validity is already checked by the TLS handshake when using tls.VerifyClientCertIfGiven.
func RequireClientCert(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	err := verifyClientCertPresence(ctx)
	if err != nil {
		return nil, err
	}

	// TLS handshake has already verified the certificate, so we can proceed
	return handler(ctx, req)
}

// RequireClientCertStream creates a gRPC stream interceptor that ensures clients provide a certificate.
// The certificate's validity is already checked by the TLS handshake when using tls.VerifyClientCertIfGiven.
func RequireClientCertStream(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	err := verifyClientCertPresence(ss.Context())
	if err != nil {
		return err
	}

	// TLS handshake has already verified the certificate, so we can proceed
	return handler(srv, ss)
}

// verifyClientCertPresence checks if the client has provided a certificate in the TLS handshake.
// It returns an error if the certificate is not present or if the request is not using TLS.
func verifyClientCertPresence(ctx context.Context) error {
	// Extract peer information from context
	p, ok := peer.FromContext(ctx)
	if !ok {
		return status.Errorf(codes.Internal, "peer information not found in request context")
	}

	// Extract TLS information
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return status.Errorf(codes.Unauthenticated, "request is not using TLS")
	}

	// Verify that client provided a certificate
	if len(tlsInfo.State.PeerCertificates) == 0 {
		return status.Errorf(codes.Unauthenticated, "client did not provide a certificate")
	}

	return nil
}
