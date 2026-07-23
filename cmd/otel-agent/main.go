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
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	_ "github.com/DataDog/datadog-agent/pkg/version"
)

const otelAgentIdentity = "otel-agent"

func initializeProcessIdentity() {
	flavor.SetFlavor(flavor.OTelAgent)
	metrics.SetAgentIdentity(otelAgentIdentity)
}

func main() {
	initializeProcessIdentity()
	os.Exit(runcmd.Run(command.MakeRootCommand()))
}
