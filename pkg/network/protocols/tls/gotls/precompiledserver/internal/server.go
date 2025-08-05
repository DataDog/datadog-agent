// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf && test
// +build linux_bpf,test

// Package main contains binaries to run
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var (
	addr     = flag.String("addr", "localhost", "server address")
	port     = flag.String("port", "8443", "server port")
	certFile = flag.String("cert", "cert.pem", "TLS certificate file")
	keyFile  = flag.String("key", "key.pem", "TLS key file")
)

func main() {
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "Hello from static HTTPS server built with Go %s\n", runtime.Version())
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	})
	mux.HandleFunc("/status/", func(w http.ResponseWriter, r *http.Request) {
		// Extract status code from path /status/{code}/...
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/status/"), "/")
		if len(parts) > 0 {
			if code := parts[0]; code != "" {
				if statusCode := parseStatusCode(code); statusCode > 0 {
					w.WriteHeader(statusCode)
					fmt.Fprintf(w, "Status %d response\n", statusCode)
					return
				}
			}
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "Default status response\n")
	})

	serverAddr := *addr + ":" + *port
	server := &http.Server{
		Addr:         serverAddr,
		Handler:      mux,
		TLSConfig:    &tls.Config{},
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		// Disabling http2
		TLSNextProto: make(map[string]func(*http.Server, *tls.Conn, http.Handler)),
	}

	// Create listener with SO_REUSEADDR
	ln, err := net.Listen("tcp", serverAddr)
	if err != nil {
		panic(err)
	}

	// Enable SO_REUSEADDR on the socket
	if tcpLn, ok := ln.(*net.TCPListener); ok {
		file, err := tcpLn.File()
		if err == nil {
			_ = syscall.SetsockoptInt(int(file.Fd()), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
			_ = file.Close()
		}
	}

	fmt.Printf("Starting HTTPS server on %s\n", serverAddr)
	err = server.ServeTLS(ln, *certFile, *keyFile)
	if err != nil {
		panic(err)
	}
}

func parseStatusCode(code string) int {
	if statusCode, err := strconv.Atoi(code); err == nil && statusCode >= 100 && statusCode < 600 {
		return statusCode
	}
	return 0
}
