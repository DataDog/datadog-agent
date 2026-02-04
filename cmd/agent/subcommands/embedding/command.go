// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package embedding implements 'agent embedding [text]'.
package embedding

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	"github.com/DataDog/datadog-agent/pkg/hello"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams
	text string
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	embeddingCmd := &cobra.Command{
		Use:   "embedding [text]",
		Short: "Print hello world using C++ function",
		Long:  `This command demonstrates calling a C++ function from Go to print hello world.`,
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				cliParams.text = args[0]
			}
			return fxutil.OneShot(runEmbedding,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot(command.LoggerName, "off", true),
				}),
				core.Bundle(),
				secretsnoopfx.Module(),
			)
		},
	}

	return []*cobra.Command{embeddingCmd}
}

func runEmbedding(_ log.Component, params *cliParams) error {
	// Call the C++ function to print hello world
	hello.PrintHelloWorld()

	// If text argument was provided, print it as well
	if params.text != "" {
		fmt.Fprintf(os.Stdout, "Text argument: %s\n", params.text)
	}

	return nil
}
