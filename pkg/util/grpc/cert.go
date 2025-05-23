// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package grpcutil provides utility functions for working with gRPC.
package grpc

import (
	"context"
	"crypto/x509"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

// CertValidatorFunc is a function that validates a client certificate.
// It should return nil if the certificate is valid, or an error if validation fails.
type CertValidatorFunc func(*x509.Certificate) error

// RequireClientCert creates a gRPC unary interceptor that ensures clients provide a certificate.
// The certificate's validity is already checked by the TLS handshake when using tls.VerifyClientCertIfGiven.
func RequireClientCert(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	// Extract peer information from context
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "peer information not found in request context")
	}

	// Extract TLS information
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, status.Errorf(codes.Unauthenticated, "request is not using TLS")
	}

	// Verify that client provided a certificate
	if len(tlsInfo.State.PeerCertificates) == 0 {
		return nil, status.Errorf(codes.Unauthenticated, "client did not provide a certificate")
	}

	// TLS handshake has already verified the certificate, so we can proceed
	return handler(ctx, req)
}

// RequireClientCertStream creates a gRPC stream interceptor that ensures clients provide a certificate.
// The certificate's validity is already checked by the TLS handshake when using tls.VerifyClientCertIfGiven.
func RequireClientCertStream(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	ctx := ss.Context()

	// Extract peer information from context
	p, ok := peer.FromContext(ctx)
	if !ok {
		return status.Errorf(codes.Unauthenticated, "peer information not found in request context")
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

	// TLS handshake has already verified the certificate, so we can proceed
	return handler(srv, ss)
}
