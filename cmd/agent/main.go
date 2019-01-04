// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !windows,!android

//go:generate go run ../../pkg/config/render_config.go agent ../../pkg/config/config_template.yaml ./dist/datadog.yaml
//go:generate go run ../../pkg/config/render_config.go network-tracer ../../pkg/config/config_template.yaml ./dist/network-tracer.yaml

package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/app"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	reaper "github.com/ramr/go-reaper"
)

func main() {
	if common.IsDockerRunning() {
		// Reap orphaned child processes
		reaperCfg := reaper.Config{
			Pid:              0, //wait for child'd process group ID is equal to that of the calling process.
			Options:          0,
			DisablePid1Check: true, // we will not be pid 1 with s6
		}
		go reaper.Start(reaperCfg)
	}

	// Invoke the Agent
	if err := app.AgentCmd.Execute(); err != nil {
		os.Exit(-1)
	}
}
