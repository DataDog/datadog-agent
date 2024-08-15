// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

// Package namedpipecmd is the entrypoint for the NamedPipeCmd tool
package namedpipecmd

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/api/util"
	winio "github.com/Microsoft/go-winio"
)

func usage() {
	fmt.Println("Usage: NamedPipeCmd.exe <GET | POST> <URI path> [POST Payload]")
	fmt.Println("  Example: NamedPipeCmd.exe GET /debug/stats")
	fmt.Println()
}

func exitWithError(err error) {
	fmt.Printf("\nError: %s\n", err.Error())
	os.Exit(1)
}

func main() {
	if len(os.Args) < 3 {
		usage()
		return
	}

	// This should match SystemProbeProductionPipeName in
	// "github.com/DataDog/datadog-agent/pkg/process/net"
	pipePath := `\\.\pipe\dd_system_probe`
	fmt.Printf("Connecting to named pipe %s ... ", pipePath)

	// The Go wrapper for named pipes does not expose buffer size for the client.
	// It seems the client named pipe is fixed with 4K bytes for its buffer.
	pipeClient, err := winio.DialPipe(pipePath, nil)
	if err != nil {
		exitWithError(err)
		return
	}

	fmt.Println("connected")

	defer func() {
		fmt.Printf("\nClosing named pipe...\n")
		pipeClient.Close()

	}()

	// The HTTP client still needs the URL as part of the request even though
	// the underlying transport is a named pipe.
	method := os.Args[1]
	uriPath := os.Args[2]
	url := "http://localhost" + uriPath

	if (method != "GET") && (method != "POST") {
		fmt.Printf("Invalid HTTP method: %s\n", method)
		os.Exit(1)
	}

	// This HTTP client handles formatting and chunked messages.
	fmt.Printf("Creating HTTP client. ")
	httpClient := http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:    2,
			IdleConnTimeout: 30 * time.Second,
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return pipeClient, nil
			},
			TLSHandshakeTimeout:   1 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 50 * time.Millisecond,
		},
	}

	fmt.Printf("Setting up options. ")

	options := &util.ReqOptions{Conn: 1}
	if options.Authtoken == "" {
		options.Authtoken = util.GetAuthToken()
	}

	if options.Ctx == nil {
		options.Ctx = context.Background()
	}

	fmt.Printf("Creating request. \n")

	req, err := http.NewRequestWithContext(options.Ctx, method, url, nil)
	if err != nil {
		exitWithError(err)
	}

	// Set required headers.
	req.Header.Set("User-Agent", "namedpipecmd/1.1")
	req.Header.Set("Accept-Encoding", "gzip")
	if options.Conn == 1 {
		req.Close = true
	}

	fmt.Printf("Sending HTTP request...\n")

	result, err := httpClient.Do(req)
	if err != nil {
		exitWithError(err)
	}

	body, err := io.ReadAll(result.Body)
	result.Body.Close()
	if err != nil {
		exitWithError(err)
	}

	fmt.Printf("Received %d bytes. Status %d\n", len(body), result.StatusCode)

	fmt.Printf("\n---------------------------------------\n\n")
	fmt.Printf("%s\n", body)
}
