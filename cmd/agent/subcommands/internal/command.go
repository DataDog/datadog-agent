// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package internal implements 'agent internal'.
package internal

import (
	"fmt"
	"io"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

// Commands returns a slice of subcommands for the 'agent internal' command.
func Commands(_ *command.GlobalParams) []*cobra.Command {
	c := &cobra.Command{
		Use:   "internal",
		Short: "Internal agent commands",
	}

	telemetryCmd := &cobra.Command{
		Use:   "telemetry",
		Short: "Print the telemetry metrics exposed by the agent",
		RunE: func(_ *cobra.Command, _ []string) error {
			payload, err := queryAgentTelemetry()
			if err != nil {
				return err
			}
			fmt.Print(string(payload))
			return nil
		},
	}
	c.AddCommand(telemetryCmd)

	return []*cobra.Command{c}
}

// queryAgentTelemetry gets the telemetry payload exposed by the agent
func queryAgentTelemetry() ([]byte, error) {
	r, err := http.Get(fmt.Sprintf("http://localhost:%s/telemetry", pkgconfigsetup.Datadog().GetString("expvar_port")))
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}
