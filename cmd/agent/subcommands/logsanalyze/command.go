// Package logscheck implements 'agent logs-analyze'.
package logscheck

import (
	"fmt"

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

// CliParams holds the command-line argument and dependencies for the logs-analyze subcommand.
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
		Use:   "logs-analyze",
		Short: "Analyze logs configuration in isolation",
		Long:  `Run the Datadog agent in logs check mode with a specific configuration file.`,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("log config file path is required")
			}
			cliParams.LogConfigPath = args[0]
			return fxutil.OneShot(runLogsAnalyze,
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

// runLogsAnalyze initializes the launcher and sends the log config file path to the source provider.
func runLogsAnalyze(config config.Component, cliParams *CliParams) error {
	agentimpl.SetUpLaunchers(config)

	//send paths to source provider
	if err := cliParams.ConfigSource.AddFileSource(cliParams.LogConfigPath); err != nil {
		return fmt.Errorf("failed to add log config source: %w", err)
	}

	// Add core config source
	if err := cliParams.ConfigSource.AddFileSource(cliParams.CoreConfigPath); err != nil {
		return fmt.Errorf("failed to add core config source: %w", err)
	}

	return nil
}
