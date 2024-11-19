// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ipc implement a basic Agent DNS to resolve Agent IPC addresses
// It would provide Client and Server building blocks to convert "http://core-cmd/agent/status" into "http://localhost:5001/agent/status" based on the configuration
package ipc

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ClientOptionCb allows configuration of the *http.Client during construction
type ClientOptionCb func(*clientParams)

type clientParams struct {
	transport *resolverRoundTripper
	Timeout   time.Duration
}

// GetClient return an http.Client with an injected resolver
// By default the resolver is a ConfigResolver
func GetClient(options ...ClientOptionCb) *http.Client {
	params := clientParams{
		transport: &resolverRoundTripper{resolver: NewConfigResolver()},
	}

	for _, opt := range options {
		opt(&params)
	}

	return &http.Client{
		Transport: params.transport,
		Timeout:   params.Timeout,
	}
}

// WithNoVerify configures the client to skip TLS verification.
func WithNoVerify() func(r *clientParams) {
	return func(r *clientParams) {
		r.transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
}

// WithTLSConfig configures the client transport with the provided TLS configuration (if the endpoint support it).
func WithTLSConfig(tlsConfig *tls.Config) func(r *clientParams) {
	return func(r *clientParams) {
		r.transport.TLSClientConfig = tlsConfig
	}
}

// WithTimeout sets the timeout for the HTTP client.
func WithTimeout(to time.Duration) func(r *clientParams) {
	return func(r *clientParams) {
		r.Timeout = to
	}
}

// WithFallbackOn authorize the http.Client to try the next Endpoint if the first one failed
func WithFallbackOn() func(r *clientParams) {
	return func(r *clientParams) {
		r.transport.withFallback = true
	}
}

// WithStrictMode doesn't try to contact the given host if it's not find in the resolver
func WithStrictMode() func(r *clientParams) {
	return func(r *clientParams) {
		r.transport.withStrictMode = true
	}
}

// WithClientResolver replace the default Configuration resolver by a provided one
func WithClientResolver(resolver AddrResolver) func(r *clientParams) {
	return func(r *clientParams) {
		r.transport.resolver = resolver
	}
}

type resolverRoundTripper struct {
	resolver        AddrResolver
	TLSClientConfig *tls.Config

	// if set to true the roundtripper will not try to dial an endpoint not present in the resolver
	withStrictMode bool

	// if set to true the roundtripper will contact sequencially the different endpoint until receiving a valid response (only if the resolver return multiple endpoints)
	withFallback bool
}

// RoundTrip function is used
// RoundTrip executes a single HTTP transaction, returning
// a Response for the provided Request. It uses a custom resolver
// to resolve the request's host to a list of endpoints and attempts
// to send the request to each endpoint in sequence. If all attempts
// fail and fallback is enabled, it falls back to the default transport.
func (r *resolverRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	endpoints, err := r.resolver.Resolve(req.Host)
	if err != nil && r.withStrictMode {
		return nil, err
	}

	if len(endpoints) == 0 && r.withStrictMode {
		return nil, fmt.Errorf("unable to resolve addr: nil pointer netAddr")
	}

	for idx, endpoint := range endpoints {
		resp, err := endpoint.RoundTripper(r.TLSClientConfig).RoundTrip(req)
		if err != nil {
			log.Debugf("request %v resolved to %v failed (endpoint %d): %s", req.Host, endpoint.Addr(), idx, err.Error())
			if !r.withFallback {
				break
			}
			continue
		}
		return resp, err
	}

	log.Debugf("unable to reach resolved endpoint, fallback to default transport")

	tr := http.Transport{
		TLSClientConfig: r.TLSClientConfig,
	}

	return tr.RoundTrip(req)
}
