// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main is a simple client for the gotls_server.
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
)

func main() {
	useHTTP2 := flag.Bool("http2", false, "enable HTTP2")
	flag.Parse()

	args := flag.Args()
	if len(args) < 2 {
		log.Fatalf("usage: %s <server_addr> <number_of_requests> [optional -http2]", os.Args[0])
	}

	serverAddr := args[0]
	reqCount, err := strconv.Atoi(args[1])
	if err != nil || reqCount < 0 {
		log.Fatalf("invalid value %q for number of requests", args[1])
	}

	transport := &http.Transport{
		ForceAttemptHTTP2: *useHTTP2,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	if !*useHTTP2 {
		transport.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
	}

	client := http.Client{
		Transport: transport,
	}

	defer client.CloseIdleConnections()
	in := make([]byte, 1)
	_, err = io.ReadFull(os.Stdin, in)
	if err != nil {
		log.Fatalf("could not read from stdin: %s", err)
	}

	for i := 0; i < reqCount; i++ {
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://%s/%d/request-%d", serverAddr, http.StatusOK, i), nil)
		if err != nil {
			log.Fatalf("could not generate HTTP request: %s", err)
		}

		resp, err := client.Do(req)
		if err != nil {
			log.Fatalf("could not do HTTPS request: %s", err)
		}

		_, err = io.Copy(io.Discard, resp.Body)
		if err != nil {
			log.Fatalf("could not read response body: %s", err)
		}

		resp.Body.Close()
	}

}
