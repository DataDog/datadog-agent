// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf && test

// Package main is the entry point for the event-viewer utility for visualizing GPU collected events
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/DataDog/datadog-agent/pkg/gpu/testutil"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("Please provide a file path")
	}

	filePath := os.Args[1]

	events, err := testutil.NewEventCollection(filePath)
	if err != nil {
		log.Fatalf("Error parsing events file: %v", err)
	}

	fmt.Printf("Parsed %d events\n", len(events.Events))

	err = events.OutputEvents(os.Stdout)
	if err != nil {
		log.Fatalf("Error outputting events: %v", err)
	}
}
