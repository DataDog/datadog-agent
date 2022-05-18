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
	"fmt"
	"net/http/httptrace"

	"github.com/fatih/color"
)

// During a request, the http.Client will call the functions of the ClientTrace at specific moments.
// This is useful to get extra information about what is happening and if there are errors during
// connection establishment, DNS resolution or TLS handshake for instance.
var DiagnoseTrace = &httptrace.ClientTrace{

	// Hooks for connection establishment
	ConnectStart: connectStartHook,
	ConnectDone:  connectDoneHook,
}

// connectStartHook is called when the http.Client is establishing a new connection to 'addr'
// However, it is not called when a connection is reused.
func connectStartHook(network, addr string) {
	fmt.Printf("~~~ Starting a new connection ~~~\n")
}

// connectDoneHook is called when the new connection to 'addr' completes
// It displays the error message if there is one and indicates if this step was successful
func connectDoneHook(network, addr string, err error) {
	statusString := color.GreenString("OK")
	if err != nil {
		statusString = color.RedString("KO")
		fmt.Printf("Unable to connect to the endpoint : %v\n", err)
	}
	fmt.Printf("Connection to the endpoint [%v]\n\n", statusString)

}
