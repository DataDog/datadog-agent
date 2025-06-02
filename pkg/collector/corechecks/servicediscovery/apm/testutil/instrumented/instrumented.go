// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package main is a go application which use dd-trace-go, in order to test
// static APM instrumentation detection. This program is never executed.
package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

func main() {
	tracer.Start()

	signalChan := make(chan os.Signal, 1)
	done := make(chan bool, 1)

	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	go func() {
		<-signalChan
		done <- true
	}()

	fmt.Println("Running... Press Ctrl+C to exit.")

	<-done // Block until a signal is received
	fmt.Println("Gracefully shutting down.")
}
