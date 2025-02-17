// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

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

	events, err := testutil.ParseEventsFile(filePath)
	if err != nil {
		log.Fatalf("Error parsing events file: %v", err)
	}

	lastTimestamp := uint64(0)
	for i, ev := range events {
		evStr, err := testutil.EventToString(ev, lastTimestamp)
		if err != nil {
			log.Fatalf("Error converting event to string: %v", err)
		}

		lastTimestamp = testutil.GetEventTimestamp(ev)
		fmt.Printf("%d: %s\n", i, evStr)
	}
}
