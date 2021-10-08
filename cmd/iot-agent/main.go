// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !windows,!android

package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/app"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func main() {
	// Set the flavor
	flavor.SetFlavor(flavor.IotAgent)

	// Invoke the Agent
	if err := app.AgentCmd.Execute(); err != nil {
		log.Error(err)
		os.Exit(-1)
	}
}
