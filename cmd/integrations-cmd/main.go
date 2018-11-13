// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build windows

package main

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/cmd/integrations-cmd/app"
)

func main() {
	fmt.Printf("Hello, integrations\n")
	// Invoke the Agent
	if err := app.AgentCmd.Execute(); err != nil {
		os.Exit(-1)
	}
}
