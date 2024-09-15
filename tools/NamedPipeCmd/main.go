// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build windows

// Package namedpipecmd is the entrypoint for the NamedPipeCmd tool
package namedpipecmd

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/api/util"
	winio "github.com/Microsoft/go-winio"
)

var (
	quiet = flag.Bool("quiet", false, "Only return the exit code on failure, or the JSON output on success")
)

func exitWithError(err error) {
	fmt.Printf("\nError: %s\n", err.Error())
	os.Exit(1)
}

func exitWithErrorCode(err int) {
	fmt.Printf("Error: %d\n", err)
	os.Exit(err)
}

func fprintf(format string, a ...interface{}) {
	if(!*quiet) {
		fmt.Printf(format, a...)
	}
}

func main() {

	method := flag.String("method", "", "GET or POST")
	path := flag.String("path", "", "URI path")
	//payload := flag.String(payload, "", "POST payload")
	flag.Parse()

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

	fprintf("connected")

	defer func() {
		fprintf("\nClosing named pipe...\n")
		pipeClient.Close()

	}()

	// The HTTP client still needs the URL as part of the request even though
	// the underlying transport is a named pipe.
	method := os.Args[1]
	uriPath := os.Args[2]
	url := "http://localhost" + uriPath

	if (*method != "GET") && (*method != "POST") {
		fprintf("Invalid HTTP method: %s\n", *method)
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

	fprintf("Setting up options. ")

	options := &util.ReqOptions{Conn: 1}
	if options.Authtoken == "" {
		options.Authtoken = util.GetAuthToken()
	}

	if options.Ctx == nil {
		options.Ctx = context.Background()
	}

	fprintf("Creating request. \n")

	req, err := http.NewRequestWithContext(options.Ctx, *method, url, nil)
	if err != nil {
		exitWithError(err)
	}

	// Set required headers.
	req.Header.Set("User-Agent", "namedpipecmd/1.1")
	req.Header.Set("Accept-Encoding", "gzip")
	if options.Conn == 1 {
		req.Close = true
	}

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

	fprintf("Received %d bytes. Status %d\n", len(body), result.StatusCode)

	fmt.Printf("\n---------------------------------------\n\n")
	fmt.Printf("%s\n", body)
}
