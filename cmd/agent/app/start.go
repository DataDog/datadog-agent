// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package app

import (
	"github.com/spf13/cobra"
)
import "github.com/DataDog/datadog-agent/pkg/traceinit"


var (
	startCmd = &cobra.Command{
		Use:        "start",
		Deprecated: "Use \"run\" instead to start the Agent",
		RunE:       start,
	}
)

func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\agent\app\start.go 20`)
	// attach the command to the root
	AgentCmd.AddCommand(startCmd)
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\agent\app\start.go 22`)

	// local flags
	startCmd.Flags().StringVarP(&pidfilePath, "pidfile", "p", "", "path to the pidfile")
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\agent\app\start.go 25`)
}

// Start the main loop
func start(cmd *cobra.Command, args []string) error {
	return run(cmd, args)
}