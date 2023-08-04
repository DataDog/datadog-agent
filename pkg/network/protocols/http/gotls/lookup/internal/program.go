// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build ignore

package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"os"
)

func main() {
	err := run()
	if err != nil {
		fmt.Printf("error: %s", err)
		os.Exit(1)
	}

	os.Exit(0)
}

const request string = "GET /status/200 HTTP/1.0\r\n\r\n"

func run() error {
	host := "httpbin.org:443"
	conn, err := tls.Dial("tcp", host, nil)
	if err != nil {
		return fmt.Errorf("could not initialize TLS connection to %q: %w", host, err)
	}

	// Send the request
	requestBuf := []byte(request)
	_, err = conn.Write(requestBuf)
	if err != nil {
		return fmt.Errorf("could not send request data to TLS connection: %w", err)
	}

	// Receive the response
	buf := make([]byte, 1024)
	_, err = conn.Read(buf)
	if err != nil && err != io.EOF {
		return fmt.Errorf("could not read response data from TLS connection: %w", err)
	}

	err = conn.Close()
	if err != nil {
		return fmt.Errorf("could not close TLS connection %w", err)
	}

	return nil
}
