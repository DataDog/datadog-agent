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

	"github.com/fatih/color"
)

// createDiagnoseTrace creates a httptrace.ClientTrace containing functions that display
// additional information when a http.Client is sending requests
// During a request, the http.Client will call the functions of the ClientTrace at specific moments
// This is useful to get extra information about what is happening and if there are errors during
// connection establishment, DNS resolution or TLS handshake for instance
func createDiagnoseTrace() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{

		// Hooks called before and after creating or retrieving a connection
		GetConn: getConnHook,
		GotConn: gotConnHook,

		// Hooks for connection establishment
		ConnectStart: connectStartHook,
		ConnectDone:  connectDoneHook,

		// Hooks for DNS resolution
		DNSStart: dnsStartHook,
		DNSDone:  dnsDoneHook,

		// Hooks for TLS Handshake
		TLSHandshakeStart: tlsHandshakeStartHook,
		TLSHandshakeDone:  tlsHandshakeDoneHook,
	}
}

var (
	dnsColorFunc = color.MagentaString
	tlsColorFunc = color.YellowString
)

// connectStartHook is called when the http.Client is establishing a new connection to 'addr'
// However, it is not called when a connection is reused (see gotConnHook)
func connectStartHook(network, addr string) {
	fmt.Printf("~~~ Starting a new connection ~~~\n")
}

// connectDoneHook is called when the new connection to 'addr' completes
// It displays the error message if there is one and indicates if this step was successful
func connectDoneHook(network, addr string, err error) {
	statusString := color.GreenString("OK")
	if err != nil {
		statusString = color.RedString("ERROR")
		fmt.Printf("Unable to connect to the endpoint : %v\n", err)
	}
	fmt.Printf("* Connection to the endpoint [%v]\n\n", statusString)
}

// getConnHook is called before getting a new connection.
// This is called before :
// 		- Creating a new connection 		: getConnHook ---> connectStartHook
//		- Retrieving an existing connection : getConnHook ---> gotConnHook
func getConnHook(hostPort string) {
	fmt.Printf("=== Retrieving or creating a new connection ===\n")
}

// gotConnHook is called after a successful connection is obtained.
// It can be called after :
// 		- New connection created 		: connectDoneHook ---> gotDoneHook
// 		- Previous connection retrieved : getConnHook     ---> gotConnHook
// This function only displays when a connection is retrieved.
// Information about new connection are reported by connectDoneHook
func gotConnHook(gci httptrace.GotConnInfo) {
	if gci.Reused {
		fmt.Print(color.CyanString("Reusing a previous connection that was idle for %v\n", gci.IdleTime))
	}
}

// dnsStartHook is called when starting the DNS lookup
func dnsStartHook(di httptrace.DNSStartInfo) {
	fmt.Print(dnsColorFunc("--- Starting DNS lookup to resolve '%v' ---\n", di.Host))
}

// dnsDoneHook is called after the DNS lookup
// It displays the error message if there is one and indicates if this step was successful
func dnsDoneHook(di httptrace.DNSDoneInfo) {
	statusString := color.GreenString("OK")
	if di.Err != nil {
		statusString = color.RedString("ERROR")
		fmt.Print(dnsColorFunc("Unable to resolve the address : %v\n", di.Err))
	}
	fmt.Printf("* %v [%v]\n\n", dnsColorFunc("DNS Lookup"), statusString)
}

// tlsHandshakeStartHook is called when starting the TLS Handshake
func tlsHandshakeStartHook() {
	fmt.Print(tlsColorFunc("### Starting TLS Handshake ###\n"))
}

// tlsHandshakeDoneHook is called after the TLS Handshake
// It displays the error message if there is one and indicates if this step was successful
func tlsHandshakeDoneHook(cs tls.ConnectionState, err error) {
	statusString := color.GreenString("OK")
	if err != nil {
		statusString = color.RedString("ERROR")
		fmt.Print(tlsColorFunc("Unable to achieve the TLS Handshake : %v\n", err))
	}
	fmt.Printf("* %v [%v]\n\n", tlsColorFunc("TLS Handshake"), statusString)
}
