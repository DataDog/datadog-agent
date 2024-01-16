// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package main provides a unix transparent proxy server that can be used for testing.
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/network/tracer/testutil/proxy"
)

func main() {
	// Define command-line flags
	var remoteAddr string
	var unixPath string
	var useTLS bool

	flag.StringVar(&remoteAddr, "remote", "", "Remote server address to forward connections to")
	flag.StringVar(&unixPath, "unix", "/tmp/transparent.sock", "A local unix socket to listen on")
	flag.BoolVar(&useTLS, "tls", false, "Use TLS to connect to the remote server")

	// Parse command-line flags
	flag.Parse()

	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGINT)

	srv := proxy.NewUnixTransparentProxyServer(unixPath, remoteAddr, useTLS)
	defer srv.Stop()

	if err := srv.Run(); err != nil {
		log.Fatal(err)
	}

	<-done
}
