// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ipc implement a basic Agent DNS to resolve Agent IPC addresses
// It would provide Client and Server building blocks to convert "http://core-cmd/agent/status" into "http://localhost:5001/agent/status" based on the configuration
package ipc

import (
	"crypto/tls"
	"net"
	"net/http"
)

// Endpoint is an abstraction of a communication channel endpoint
type Endpoint interface {
	// RoundTripper is a lazy http.RoundTripper getter, to use with http.Client
	//   client := http.Client{
	//   	Transport: RoundTripper()
	//   }
	RoundTripper(*tls.Config) http.RoundTripper

	// Listener is a lazy net.Listener getter, to use with http.Server
	//   server := http.Server{
	//   server.Serve(Listener())
	Listener() (net.Listener, error)

	// Addr return a string representation of the endpoint location
	Addr() string
}

// AddrResolver is a phonebook used to store correspondences between human-friendly addresses (such as "core-cmd") and their dynamic address getters.
type AddrResolver interface {
	// Resolve convert a generic endpoint address into a slice of endpoints
	Resolve(addr string) ([]Endpoint, error)
}

// The following constant values represent the Agent generic names
const (
	CoreCmd        = "core-cmd"        // CoreCmd is the core Agent command endpoint
	CoreIPC        = "core-ipc"        // CoreIPC is the core Agent configuration synchronisation endpoint
	CoreExpvar     = "core-expvar"     // CoreExpvar is the core Agent expvar endpoint
	TraceCmd       = "trace-cmd"       // TraceCmd is the trace Agent command endpoint
	TraceExpvar    = "trace-expvar"    // TraceExpvar is the trace Agent expvar endpoint
	SecurityCmd    = "security-cmd"    // SecurityCmd is the security Agent command endpoint
	SecurityExpvar = "security-expvar" // SecurityExpvar is the security Agent expvar endpoint
	ProcessCmd     = "process-cmd"     // ProcessCmd is the process Agent command endpoint
	ProcessExpvar  = "process-expvar"  // ProcessExpvar is the process Agent expvar endpoint
	ClusterAgent   = "cluster-agent"   // ClusterAgent is the Cluster Agent command endpoint
)

// AddrGetter is a dynamic address getter that returns a slice of addresses where the distant resources can be reached.
// For example, "core-cmd" -> {"tcp", "127.0.0.1:5001"}.
type AddrGetter func() ([]Endpoint, error)
