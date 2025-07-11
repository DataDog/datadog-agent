// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package connectivity contains logic for connectivity troubleshooting between the Agent
// and Datadog endpoints. It uses HTTP request to contact different endpoints and displays
// some results depending on endpoints responses, if any.
package connectivity

// This file contains all the functions used by httptrace.ClientTrace.
// Each function is called at a specific moment during the communication
// Their prototypes are defined by htpp.Client so variables might be unused

import (
	"crypto/tls"
	"fmt"
	"net/http/httptrace"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// createDiagnoseTraces creates a httptrace.ClientTrace containing functions that collects
// additional information when a http.Client is sending requests
// During a request, the http.Client will call the functions of the ClientTrace at specific moments
// This is useful to get extra information about what is happening and if there are errors during
// connection establishment, DNS resolution or TLS handshake for instance
// errorOnly is used to only collect information when there is an error
func createDiagnoseTraces(httpTraces *[]string, errorOnly bool) *httptrace.ClientTrace {

	hooks := &httpTraceContext{
		httpTraces: httpTraces,
		errorOnly:  errorOnly,
	}

	if errorOnly {
		return &httptrace.ClientTrace{
			// Hooks for connection establishment
			ConnectDone: hooks.connectDoneHook,

			// Hooks for DNS resolution
			DNSDone: hooks.dnsDoneHook,

			// Hooks for TLS Handshake
			TLSHandshakeDone: hooks.tlsHandshakeDoneHook,
		}
	}

	return &httptrace.ClientTrace{

		// Hooks called before and after creating or retrieving a connection
		GetConn: hooks.getConnHook,
		GotConn: hooks.gotConnHook,

		// Hooks for connection establishment
		ConnectStart: hooks.connectStartHook,
		ConnectDone:  hooks.connectDoneHook,

		// Hooks for DNS resolution
		DNSStart: hooks.dnsStartHook,
		DNSDone:  hooks.dnsDoneHook,

		// Hooks for TLS Handshake
		TLSHandshakeStart: hooks.tlsHandshakeStartHook,
		TLSHandshakeDone:  hooks.tlsHandshakeDoneHook,
	}
}

// httpTraceContext collect reported HTTP traces into its holding array
// to be retrieved later by client
type httpTraceContext struct {
	httpTraces *[]string
	errorOnly  bool
}

// connectStartHook is called when the http.Client is establishing a new connection to 'addr'
// However, it is not called when a connection is reused (see gotConnHook)
func (c *httpTraceContext) connectStartHook(_, _ string) {
	if !c.errorOnly {
		*(c.httpTraces) = append(*(c.httpTraces), "...Starting a new connection")
	}
}

// connectDoneHook is called when the new connection to 'addr' completes
// It collects the error message if there is one and indicates if this step was successful
func (c *httpTraceContext) connectDoneHook(_, _ string, err error) {
	if err != nil {
		*(c.httpTraces) = append(*(c.httpTraces), fmt.Sprintf("...Unable to connect to the endpoint: %v", scrubber.ScrubLine(err.Error())))
	} else if !c.errorOnly {
		*(c.httpTraces) = append(*(c.httpTraces), "...Connection to the endpoint: OK")
	}
}

// getConnHook is called before getting a new connection.
// This is called before :
//   - Creating a new connection 		: getConnHook ---> connectStartHook
//   - Retrieving an existing connection : getConnHook ---> gotConnHook
func (c *httpTraceContext) getConnHook(_ string) {
	*(c.httpTraces) = append(*(c.httpTraces), "...Retrieving or creating a new connection")
}

// gotConnHook is called after a successful connection is obtained.
// It can be called after :
//   - New connection created 		: connectDoneHook ---> gotDoneHook
//   - Previous connection retrieved : getConnHook     ---> gotConnHook
//
// This function only collects information when a connection is retrieved.
// Information about new connection are reported by connectDoneHook
func (c *httpTraceContext) gotConnHook(gci httptrace.GotConnInfo) {
	if gci.Reused {
		*(c.httpTraces) = append(*(c.httpTraces), fmt.Sprintf("...Reusing a previous connection that was idle for %v", gci.IdleTime))
	}
}

// dnsStartHook is called when starting the DNS lookup
func (c *httpTraceContext) dnsStartHook(di httptrace.DNSStartInfo) {
	*(c.httpTraces) = append(*(c.httpTraces), fmt.Sprintf("...Starting DNS lookup to resolve %v", di.Host))
}

// dnsDoneHook is called after the DNS lookup
// It collects the error message if there is one and indicates if this step was successful
func (c *httpTraceContext) dnsDoneHook(di httptrace.DNSDoneInfo) {
	if di.Err != nil {
		*(c.httpTraces) = append(*(c.httpTraces), fmt.Sprintf("...Unable to resolve the address: %v", scrubber.ScrubLine(di.Err.Error())))
	} else if !c.errorOnly {
		*(c.httpTraces) = append(*(c.httpTraces), "...DNS Lookup: OK")
	}
}

// tlsHandshakeStartHook is called when starting the TLS Handshake
func (c *httpTraceContext) tlsHandshakeStartHook() {
	*(c.httpTraces) = append(*(c.httpTraces), "...Starting TLS Handshake")
}

// tlsHandshakeDoneHook is called after the TLS Handshake
// It collects the error message if there is one and indicates if this step was successful
func (c *httpTraceContext) tlsHandshakeDoneHook(_ tls.ConnectionState, err error) {
	if err != nil {
		*(c.httpTraces) = append(*(c.httpTraces), fmt.Sprintf("...Unable to achieve the TLS Handshake: %v", scrubber.ScrubLine(err.Error())))
		if !c.errorOnly {
			c.getTLSHandshakeHints(err)
		}
	} else if !c.errorOnly {
		*(c.httpTraces) = append(*(c.httpTraces), "...TLS Handshake: OK")
	}
}

// getTLSHandshakeHints is called when the TLS handshake fails.
// It aims to give more context on why the handshake failed when the error is not clear enough.
func (c *httpTraceContext) getTLSHandshakeHints(err error) {
	if strings.Contains(err.Error(), "first record does not look like a TLS handshake") {
		*(c.httpTraces) = append(*(c.httpTraces), fmt.Sprintf("...%s %s",
			"Hint: you are trying to communicate using HTTPS with an endpoint that does not seem to be configured for HTTPS.",
			"If you are using a proxy, please verify that it is configured for HTTPS connections."))
	}
}
