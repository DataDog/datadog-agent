// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package httpclient

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"golang.org/x/net/http2"
)

// Authorizer describes security authorization gates for HTTP clients.
type Authorizer interface {
	// IsNetworkAddressAuthorized returns true if the given network/address
	// tuple is allowed.
	IsNetworkAddressAuthorized(network, address string) (bool, error)
}

// WrapClient wraps an HTTP client with security controls
func WrapClient(client *http.Client, az Authorizer) *http.Client {
	if client == nil {
		return nil
	}

	defaultTransport := &http.Transport{
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	dialer := &net.Dialer{
		// Disable the fallback delay (Happy Eyeballs algorithm)
		FallbackDelay: -1 * time.Millisecond,
		// see: net.DefaultResolver
		Resolver: &net.Resolver{
			// Prefer Go's built-in DNS resolver.
			PreferGo: true,
		},
	}

	dialer.Control = func(network, address string, rc syscall.RawConn) error {
		// Check network / address authorization
		if allow, err := az.IsNetworkAddressAuthorized(network, address); !allow {
			return fmt.Errorf("%s/%s is not authorized by the client: %w", network, address, err)
		}
		return nil
	}

	// From now on, the dialer has been wrapped with the safe control function.

	transport := client.Transport
	// If the transport is nil, use the default transport
	if transport == nil {
		transport = defaultTransport
	}

	// Decorate the transport with the safe dialer
	switch tr := transport.(type) {
	case *http2.Transport:
		tr.DialTLSContext = func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
			tlsDialer := &tls.Dialer{
				NetDialer: dialer,
				Config:    cfg,
			}
			return tlsDialer.DialContext(ctx, network, addr)
		}
	case *http.Transport:
		tr.DialContext = dialer.DialContext
	default:
		log.Errorf("client.Transport is of type %T, not of type *http.Transport or *http2.Transport, replacing with the default transporter", client.Transport)
		transport = defaultTransport
		transport.(*http.Transport).DialContext = dialer.DialContext
	}

	// Create a new client with the wrapped transport
	result := *client // Make a copy of the original client
	result.Transport = transport

	return &result
}
