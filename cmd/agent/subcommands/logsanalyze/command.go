// Package logscheck implements 'agent logs-analyze'.
package logscheck

import (
	"fmt"

	"go.uber.org/fx"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const defaultCoreConfigPath = "bin/agent/dist/datadog.yaml"

// CliParams holds the command-line arguments for the logs-analyze subcommand.
type CliParams struct {
	*command.GlobalParams

	// LogConfigPath represents the path to the logs configuration file.
	LogConfigPath string

	// CoreConfigPath represents the path to the core configuration file.
	CoreConfigPath string

	ConfigSource launchers.SourceProvider
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &CliParams{
		GlobalParams:   globalParams,
		CoreConfigPath: defaultCoreConfigPath, // Set default path
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
			fmt.Println("andrewq1", cliParams.LogConfigPath)
			return fxutil.OneShot(runLogsAnalyze,
				fx.Provide(sources.NewConfigSources),
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
			)
		},
	}

	// Add flag for core config (optional)
	cmd.Flags().StringVarP(&cliParams.CoreConfigPath, "core-config", "C", defaultCoreConfigPath, "Path to the core configuration file (optional)")

	return []*cobra.Command{cmd}
}

// runLogsAnalyze handles the logs check operation.
func runLogsAnalyze(cliParams *CliParams) error {
	// Check if the provided log config file exists
	// if _, err := os.Stat(cliParams.LogConfigPath); os.IsNotExist(err) {
	// 	return fmt.Errorf("log config file %s does not exist", cliParams.LogConfigPath)
	// }

	// // Check if the core config file exists, using default or user-specified path
	// if _, err := os.Stat(cliParams.CoreConfigPath); os.IsNotExist(err) {
	// 	return fmt.Errorf("core config file %s does not exist", cliParams.CoreConfigPath)
	// }

	//send paths to source provider
	// Add log config source
	fmt.Println("andrewq command.go 1")
	if err := cliParams.ConfigSource.AddFileSource(cliParams.LogConfigPath); err != nil {
		return fmt.Errorf("failed to add log config source: %w", err)
	}

	fmt.Println("andrewq command.go 2")
	// Add core config source
	if err := cliParams.ConfigSource.AddFileSource(cliParams.CoreConfigPath); err != nil {
		return fmt.Errorf("failed to add core config source: %w", err)
	}
	fmt.Println("andrewq command.go 3, added both log files")

	return nil
}
