// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Main package for the agent binary
package main

import (
	"github.com/spf13/cobra"
)

var agents = map[string]func() *cobra.Command{}

func registerAgent(names []string, getCommand func() *cobra.Command) {
	for _, name := range names {
		agents[name] = getCommand
	}
}
