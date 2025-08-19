// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main provides a simple gRPC server that can be used for testing.
package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/network/usm/testutil/grpc"
)

func main() {
	// Define command-line flags
	var addr string
	var useTLS bool

	flag.StringVar(&addr, "addr", "127.0.0.1:5050", "Address parameter")
	flag.BoolVar(&useTLS, "tls", false, "Use TLS flag")

	// Parse command-line flags
	flag.Parse()

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT)

	srv, err := grpc.NewServer(addr, useTLS)
	if err != nil {
		os.Exit(1)
	}

	srv.Run()

	<-done
}
