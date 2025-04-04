// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package main contains a portable binary to easily run SACK traceroutes
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/common"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/sack"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

func main() {
	loglevel := os.Getenv("LOG_LEVEL")
	if loglevel == "" {
		loglevel = "warn"
	}

	err := pkglogsetup.SetupLogger(
		pkglogsetup.LoggerName("sack"),
		loglevel,
		"",
		"",
		false,
		true,
		false,
		pkgconfigsetup.Datadog(),
	)
	if err != nil {
		fmt.Printf("SetupLogger failed: %s\n", err)
		os.Exit(1)
	}

	if len(os.Args) < 2 {
		println("Usage: portable_sack <target>")
		os.Exit(1)
	}
	target := os.Args[1]

	cfg := sack.Params{
		Target:           netip.MustParseAddrPort(target),
		HandshakeTimeout: 500 * time.Millisecond,
		FinTimeout:       500 * time.Millisecond,
		ParallelParams: common.TracerouteParallelParams{
			MinTTL:            1,
			MaxTTL:            30,
			TracerouteTimeout: 1 * time.Second,
			PollFrequency:     100 * time.Millisecond,
			SendDelay:         10 * time.Millisecond,
		},
		LoosenICMPSrc: true,
	}

	results, err := sack.RunSackTraceroute(context.Background(), cfg)
	if err != nil {
		fmt.Printf("Traceroute failed: %s\n", err)
		os.Exit(1)
	}
	json, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		fmt.Printf("Error marshalling results: %s\n", err)
		os.Exit(1)
	}
	println(string(json))
}
