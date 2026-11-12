// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build otlp

// otel-agent is a standalone binary that runs the OpenTelemetry Collector.
package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/internal/runcmd"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/command"
	logsmetrics "github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/mallocarena"
	_ "github.com/DataDog/datadog-agent/pkg/version"
)

const otelAgentIdentity = "otel-agent"

func initializeProcessIdentity() {
	flavor.SetFlavor(flavor.OTelAgent)
	logsmetrics.SetAgentIdentity(otelAgentIdentity)
}

// capMallocArenas bounds the per-thread arenas glibc hands out. The collector churns
// multi-megabyte blocks through cgo (zstd stream contexts, most of all) from a thread pool
// sized by CPU count, and glibc never returns a secondary arena's memory to the OS, so an
// uncapped process settles at the sum of every arena's high-water mark.
func capMallocArenas() {
	// Logging before setup is buffered and replayed once the logger exists.
	if summary, err := mallocarena.SetMax(mallocarena.DefaultMaxArenas); err != nil {
		log.Warn(summary)
	} else {
		log.Info(summary)
	}
}

func main() {
	capMallocArenas()
	initializeProcessIdentity()
	os.Exit(runcmd.Run(command.MakeRootCommand()))
}
