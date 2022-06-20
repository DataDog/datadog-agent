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
	"io"
	"net/http/httptrace"

	"github.com/fatih/color"
)

// createDiagnoseTrace creates a httptrace.ClientTrace containing functions that display
// additional information when a http.Client is sending requests
// During a request, the http.Client will call the functions of the ClientTrace at specific moments
// This is useful to get extra information about what is happening and if there are errors during
// connection establishment, DNS resolution or TLS handshake for instance
func createDiagnoseTrace(writer *writerWrapper) *httptrace.ClientTrace {

	return &httptrace.ClientTrace{

		// Hooks called before and after creating or retrieving a connection
		GetConn: writer.getConnHook,
		GotConn: writer.gotConnHook,

		// Hooks for connection establishment
		ConnectStart: writer.connectStartHook,
		ConnectDone:  writer.connectDoneHook,

		// Hooks for DNS resolution
		DNSStart: writer.dnsStartHook,
		DNSDone:  writer.dnsDoneHook,

		// Hooks for TLS Handshake
		TLSHandshakeStart: writer.tlsHandshakeStartHook,
		TLSHandshakeDone:  writer.tlsHandshakeDoneHook,
	}
}

var (
	dnsColorFunc = color.MagentaString
	tlsColorFunc = color.YellowString
)

// writeWrapper is used to pass a io.Writer variable to the hooks so that they
// can output into a terminal if the command is used via CLI or into a buffer
// when requesting a flare
type writerWrapper struct {
	w io.Writer
}

// connectStartHook is called when the http.Client is establishing a new connection to 'addr'
// However, it is not called when a connection is reused (see gotConnHook)
func (writer *writerWrapper) connectStartHook(network, addr string) {
	fmt.Fprintf(writer.w, "~~~ Starting a new connection ~~~\n")
}

// connectDoneHook is called when the new connection to 'addr' completes
// It displays the error message if there is one and indicates if this step was successful
func (writer *writerWrapper) connectDoneHook(network, addr string, err error) {
	statusString := color.GreenString("OK")
	if err != nil {
		statusString = color.RedString("ERROR")
		fmt.Fprintf(writer.w, "Unable to connect to the endpoint : %v\n", err)
	}
	fmt.Fprintf(writer.w, "* Connection to the endpoint [%v]\n\n", statusString)
}

// getConnHook is called before getting a new connection.
// This is called before :
// 		- Creating a new connection 		: getConnHook ---> connectStartHook
//		- Retrieving an existing connection : getConnHook ---> gotConnHook
func (writer *writerWrapper) getConnHook(hostPort string) {
	fmt.Fprintf(writer.w, "=== Retrieving or creating a new connection ===\n")
}

// gotConnHook is called after a successful connection is obtained.
// It can be called after :
// 		- New connection created 		: connectDoneHook ---> gotDoneHook
// 		- Previous connection retrieved : getConnHook     ---> gotConnHook
// This function only displays when a connection is retrieved.
// Information about new connection are reported by connectDoneHook
func (writer *writerWrapper) gotConnHook(gci httptrace.GotConnInfo) {
	if gci.Reused {
		fmt.Fprint(writer.w, color.CyanString("Reusing a previous connection that was idle for %v\n", gci.IdleTime))
	}
}

// dnsStartHook is called when starting the DNS lookup
func (writer *writerWrapper) dnsStartHook(di httptrace.DNSStartInfo) {
	fmt.Fprint(writer.w, dnsColorFunc("--- Starting DNS lookup to resolve '%v' ---\n", di.Host))
}

// dnsDoneHook is called after the DNS lookup
// It displays the error message if there is one and indicates if this step was successful
func (writer *writerWrapper) dnsDoneHook(di httptrace.DNSDoneInfo) {
	statusString := color.GreenString("OK")
	if di.Err != nil {
		statusString = color.RedString("ERROR")
		fmt.Fprint(writer.w, dnsColorFunc("Unable to resolve the address : %v\n", di.Err))
	}
	fmt.Fprintf(writer.w, "* %v [%v]\n\n", dnsColorFunc("DNS Lookup"), statusString)
}

// tlsHandshakeStartHook is called when starting the TLS Handshake
func (writer *writerWrapper) tlsHandshakeStartHook() {
	fmt.Fprint(writer.w, tlsColorFunc("### Starting TLS Handshake ###\n"))
}

// tlsHandshakeDoneHook is called after the TLS Handshake
// It displays the error message if there is one and indicates if this step was successful
func (writer *writerWrapper) tlsHandshakeDoneHook(cs tls.ConnectionState, err error) {
	statusString := color.GreenString("OK")
	if err != nil {
		statusString = color.RedString("ERROR")
		fmt.Fprint(writer.w, tlsColorFunc("Unable to achieve the TLS Handshake : %v\n", err))
	}
	fmt.Fprintf(writer.w, "* %v [%v]\n\n", tlsColorFunc("TLS Handshake"), statusString)
}
