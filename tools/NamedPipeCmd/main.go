// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

// Package namedpipecmd is the entrypoint for the NamedPipeCmd tool
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	winio "github.com/Microsoft/go-winio"
)

var (
	quiet = flag.Bool("quiet", false, "Only return the exit code on failure, or the JSON output on success")
)

func printUsage() {
	fmt.Printf("Usage: NamedPipeCmd.exe -method <GET | POST> -path <URI> [-quiet]\n")
	fmt.Printf("Example: NamedPipeCmd.exe -method GET -path /debug/stats\n")
}

func exitWithError(err error) {
	fmt.Printf("\nError: %s\n", err.Error())
	os.Exit(1)
}

func fprintf(format string, a ...interface{}) {
	if !*quiet {
		fmt.Printf(format, a...)
	}
}

func main() {

	if len(os.Args) < 3 {
		printUsage()
		os.Exit(1)
	}

	method := flag.String("method", "", "GET or POST")
	uriPath := flag.String("path", "", "URI path")
	flag.Parse()

	// This should match SystemProbePipeName in
	// "github.com/DataDog/datadog-agent/cmd/system-probe/client"
	pipePath := `\\.\pipe\dd_system_probe`
	fprintf("Connecting to named pipe %s ... ", pipePath)

	// The HTTP client still needs the URL as part of the request even though
	// the underlying transport is a named pipe.
	url := "http://localhost" + *uriPath

	if (*method != "GET") && (*method != "POST") {
		fprintf("Invalid HTTP method: %s\n", *method)
		os.Exit(1)
	}

	// The Go wrapper for named pipes does not expose buffer size for the client.
	// It seems the client named pipe is fixed with 4K bytes for its buffer.
	pipeClient, err := winio.DialPipe(pipePath, nil)
	if err != nil {
		exitWithError(err)
		return
	}

	fprintf("connected.\n")

	defer func() {
		fprintf("\nClosing named pipe...\n")
		pipeClient.Close()

	}()

	// This HTTP client handles formatting and chunked messages.
	fprintf("Creating HTTP client. ")
	httpClient := http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:    2,
			IdleConnTimeout: 5 * time.Second,
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return pipeClient, nil
			},
			TLSHandshakeTimeout:   1 * time.Second,
			ResponseHeaderTimeout: 5 * time.Second,
			ExpectContinueTimeout: 50 * time.Millisecond,
		},
	}

	fprintf("Creating request.\n")

	req, err := http.NewRequestWithContext(context.Background(), *method, url, nil)
	if err != nil {
		exitWithError(err)
	}

	// Set required headers.
	req.Header.Set("User-Agent", "namedpipecmd/1.1")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Close = true

	fprintf("Sending HTTP request...\n")

	result, err := httpClient.Do(req)
	if err != nil {
		exitWithError(err)
	}

	body, err := io.ReadAll(result.Body)
	result.Body.Close()
	if err != nil {
		exitWithError(err)
	}

	fprintf("Received %d bytes. Status: %d\n", len(body), result.StatusCode)

	fprintf("\n---------------------------------------\n\n")
	fmt.Printf("%s\n", body)
}
