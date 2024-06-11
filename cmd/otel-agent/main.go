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
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
)

func main() {
	flavor.SetFlavor(flavor.OTelAgent)
	os.Exit(runcmd.Run(command.MakeRootCommand()))
}
