// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package analyzelogs implements 'agent analyze-logs'.
package analyzelogs

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"go.uber.org/fx"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/logs/agent/agentimpl"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const defaultCoreConfigPath = "bin/agent/dist/datadog.yaml"

// CliParams holds the command-line argument and dependencies for the analyze-logs subcommand.
type CliParams struct {
	*command.GlobalParams

	// LogConfigPath represents the path to the logs configuration file.
	LogConfigPath string

	// CoreConfigPath represents the path to the core configuration file.
	CoreConfigPath string

	ConfigSource *sources.ConfigSources
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &CliParams{
		GlobalParams:   globalParams,
		CoreConfigPath: defaultCoreConfigPath, // Set default path
		ConfigSource:   sources.GetInstance(),
	}

	cmd := &cobra.Command{
		Use:   "analyze-logs",
		Short: "Analyze logs configuration in isolation",
		Long:  `Run the Datadog agent in logs check mode with a specific configuration file.`,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("log config file path is required")
			}
			cliParams.LogConfigPath = args[0]
			return fxutil.OneShot(runAnalyzeLogs,
				core.Bundle(),
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
			)
		},
	}

	// Add flag for core config (optional)
	cmd.Flags().StringVarP(&cliParams.CoreConfigPath, "core-config", "C", defaultCoreConfigPath, "Path to the core configuration file (optional)")

	return []*cobra.Command{cmd}
}

// runAnalyzeLogs initializes the launcher and sends the log config file path to the source provider.
func runAnalyzeLogs(cliParams *CliParams, config config.Component) error {
	outputChan := agentimpl.SetUpLaunchers(config)

	// Send paths to source provider
	if err := cliParams.ConfigSource.AddFileSource(cliParams.LogConfigPath); err != nil {
		return fmt.Errorf("failed to add log config source: %w", err)
	}

	// Add core config source
	if err := cliParams.ConfigSource.AddFileSource(cliParams.CoreConfigPath); err != nil {
		return fmt.Errorf("failed to add core config source: %w", err)
	}

	// Set up an inactivity timeout
	inactivityTimeout := 3 * time.Second
	idleTimer := time.NewTimer(inactivityTimeout)

	for {
		select {
		case msg := <-outputChan:
			// Parse the JSON content
			var parsedMessage struct {
				Message struct {
					Message string `json:"message"`
				} `json:"message"`
			}

			err := json.Unmarshal(msg.GetContent(), &parsedMessage)
			if err != nil {
				fmt.Printf("Failed to parse message: %v\n", err)
				continue
			}

			// Convert literal \n to actual newlines, this happens when the config file has processing rules
			processedMessage := strings.ReplaceAll(parsedMessage.Message.Message, `\n`, "\n")

			// Print the processed message
			fmt.Println(processedMessage)

			// Reset the inactivity timer every time a message is processed
			if !idleTimer.Stop() {
				<-idleTimer.C
			}
			idleTimer.Reset(inactivityTimeout)
		case <-idleTimer.C:
			// Timeout reached, signal quit
			return nil
		}
	}
}
