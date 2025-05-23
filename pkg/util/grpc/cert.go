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

// ClientCertValidator creates a gRPC unary interceptor that validates client certificates.
func ClientCertValidator(validator CertValidatorFunc) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
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

		// Get the client certificate
		cert := tlsInfo.State.PeerCertificates[0]

		// Validate the certificate using the provided validator function
		if err := validator(cert); err != nil {
			return nil, status.Errorf(codes.Unauthenticated, "client certificate validation failed: %v", err)
		}

		// Continue with the RPC
		return handler(ctx, req)
	}
}

// ClientCertStreamValidator creates a gRPC stream interceptor that validates client certificates.
func ClientCertStreamValidator(validator CertValidatorFunc) grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
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

		// Get the client certificate
		cert := tlsInfo.State.PeerCertificates[0]

		// Validate the certificate using the provided validator function
		if err := validator(cert); err != nil {
			return status.Errorf(codes.Unauthenticated, "client certificate validation failed: %v", err)
		}

		// Continue with the RPC
		return handler(srv, ss)
	}
}
