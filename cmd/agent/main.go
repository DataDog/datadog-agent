// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build !windows,!android

//go:generate go run ../../pkg/config/render_config.go agent-py2py3 ../../pkg/config/config_template.yaml ./dist/datadog.yaml
//go:generate go run ../../pkg/config/render_config.go agent-py3 ../../pkg/config/config_template.yaml ./dist/datadog.yaml
//go:generate go run ../../pkg/config/render_config.go system-probe ../../pkg/config/config_template.yaml ./dist/system-probe.yaml

package main

import (
	"os"

	"github.com/DataDog/datadog-agent/cmd/agent/app"
)

func main() {
	// Invoke the Agent
	if err := app.AgentCmd.Execute(); err != nil {
		os.Exit(-1)
	}
}
