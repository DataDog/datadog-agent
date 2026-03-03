// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

// Package standalone_attacher is a standalone attacher that can be used to attach probes to a process in a standalone
// process (usually inside of a container). The attacher will listen for HTTP requests and will reply with the status of
// the attached probes.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes"
)

var configName = flag.String("config", "", "config name")
var useEventStream = flag.Bool("use-event-stream", false, "use event stream")

func main() {
	// use testing.Main so that we have a testing.T to pass to the runners.
	// While this is not a real test, it's part of the test code so it makes sense,
	// We also cannot write this as a regular test because then it would be executed by the go test command
	testing.Init()
	testing.Main(func(_, _ string) (bool, error) {
		return true, nil
	}, []testing.InternalTest{
		{Name: "RunAttacher", F: run},
	}, nil, nil)
}

func run(t *testing.T) {
	runner := uprobes.NewSameProcessAttacherRunner(*useEventStream)

	if *configName == "" {
		t.Fatal("config name is required")
	}

	runner.RunAttacher(t, uprobes.AttacherTestConfigName(*configName))

	// Start HTTP server
	http.HandleFunc("/probes", func(w http.ResponseWriter, _ *http.Request) {
		probes := runner.GetProbes(t)
		err := json.NewEncoder(w).Encode(probes)
		if err != nil {
			log.Printf("failed to encode probes: %v", err)
		}
	})

	go func() {
		if err := http.ListenAndServe("0.0.0.0:8080", nil); err != nil {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	t.Logf("standalone attacher ready to serve requests, PID: %d", os.Getpid())

	// Wait for signals
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
}
