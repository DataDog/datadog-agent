// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package main provides a test program for generating lookup tables for TLS types
package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"golang.org/x/net/netutil"
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
	createTCPListener()

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

// This code is meant to include the netutil.limitedListenerConn type for the
// purposes of binary inspection. This type is sometimes behind the `net.Conn`
// interface embedded in the `tls.Conn` type.
func createTCPListener() {
	l, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal(err)
	}

	ll := netutil.LimitListener(l, 1)

	// This code is a bit non-sensical as it doesn't do anything meaninful other
	// than "including" the types we want for DWARF inspection
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		c, _ := ll.Accept()
		fmt.Println(c)
	}()
	ll.Close()
	wg.Wait()
}
