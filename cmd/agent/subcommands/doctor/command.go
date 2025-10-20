// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package doctor implements 'agent doctor'.
package doctor

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/subcommands/doctor/tui"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	// noTUI disables the interactive TUI and prints JSON instead
	noTUI              bool
	logLevelDefaultOff command.LogLevelDefaultOff
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Interactive troubleshooting dashboard for the Datadog Agent",
		Long: `Displays a real-time interactive dashboard showing:
- Ingestion status (checks, DogStatsD, logs, metrics)
- Agent health and metadata
- Backend connectivity and data forwarding status

This helps quickly identify issues with your agent setup.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(doctorCmd,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot(command.LoggerName, cliParams.logLevelDefaultOff.Value(), true),
				}),
				core.Bundle(),
				secretsnoopfx.Module(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	cliParams.logLevelDefaultOff.Register(cmd)
	cmd.Flags().BoolVar(&cliParams.noTUI, "no-tui", false, "Disable interactive TUI and print JSON instead")

	return []*cobra.Command{cmd}
}

func doctorCmd(_ log.Component, cliParams *cliParams, client ipc.HTTPClient) error {
	if cliParams.noTUI {
		// Non-interactive mode: fetch once and print JSON
		return printJSON(client)
	}

	// Interactive TUI mode
	return tui.Run(client)
}

func printJSON(client ipc.HTTPClient) error {
	endpoint, err := client.NewIPCEndpoint("/agent/doctor")
	if err != nil {
		return fmt.Errorf("unable to create IPC endpoint: %w", err)
	}

	res, err := endpoint.DoGet()
	if err != nil {
		return fmt.Errorf("unable to fetch doctor status: %w", err)
	}

	fmt.Println(string(res))
	return nil
}
