// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Command rc_tester_v1 is a test program to exercise testing against the
// remote config service. It is used for testing how the remote config service
// interacts with the dd-trace-go library (v1).
package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	httptrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/net/http"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	defer func() {
		log.Println("Stopping sample process")
	}()

	tracerEnabled := os.Getenv("DD_SERVICE") != "" &&
		os.Getenv("DD_DYNAMIC_INSTRUMENTATION_ENABLED") != "" &&
		os.Getenv("DD_DYNAMIC_INSTRUMENTATION_OFFLINE") == ""

	log.Println("Starting sample process with tracerEnabled=", tracerEnabled)

	if tracerEnabled {
		ddAgentHost := os.Getenv("DD_AGENT_HOST")
		if ddAgentHost == "" {
			ddAgentHost = "localhost"
		}
		ddAgentPort := os.Getenv("DD_AGENT_PORT")
		if ddAgentPort == "" {
			ddAgentPort = "8126"
		}
		var opts []tracer.StartOption
		if ddAgentPort != "" && ddAgentHost != "" {
			opts = append(opts, tracer.WithAgentAddr(net.JoinHostPort(ddAgentHost, ddAgentPort)))
		}
		// Start the tracer and defer the Stop method.
		tracer.Start(opts...)
		defer tracer.Stop()
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port
	fmt.Printf("Listening on port %d\n", port)

	mux := httptrace.NewServeMux()
	mux.HandleFunc("/", HandleHTTP)
	s := &http.Server{
		Handler:     mux,
		ReadTimeout: 10 * time.Second, // the linter told me to
	}
	go s.Serve(ln)

	<-ctx.Done()
	log.Println("Stopping HTTP server")
	s.Shutdown(context.Background())
}

func HandleHTTP(w http.ResponseWriter, r *http.Request) {
	span, _ := tracer.StartSpanFromContext(r.Context(), "http.handler.span")
	defer span.Finish()
	log.Printf("HandleHTTP: %v", r.URL.Path)
	LookAtTheRequest(r.URL.Path)
	w.WriteHeader(http.StatusOK)
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Hello, %s!", r.URL.Path)
	w.Write(buf.Bytes())
}

//go:noinline
func LookAtTheRequest(path string) {
	log.Printf("LookAtTheRequest: %v", path)
}
