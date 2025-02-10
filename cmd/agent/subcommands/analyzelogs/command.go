// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package analyzelogs implements 'agent analyze-logs'.
package analyzelogs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/fx"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	dualTaggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-dual"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/defaults"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/logs/agent/agentimpl"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/launchers"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pipeline"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/schedulers/ad"
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

	// inactivityTimeout represents the time in seconds that the program will wait for new logs before exiting
	inactivityTimeout int
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &CliParams{
		GlobalParams:   globalParams,
		CoreConfigPath: defaultCoreConfigPath, // Set default path
	}

	cmd := &cobra.Command{
		Use:   "analyze-logs",
		Short: "Analyze logs configuration in isolation",
		Long:  `Run a Datadog agent logs configuration and print the results to stdout`,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("log config file path is required")
			}
			cliParams.LogConfigPath = args[0]
			return fxutil.OneShot(runAnalyzeLogs,
				core.Bundle(),
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot("", "off", true)}),
				dualTaggerfx.Module(common.DualTaggerParams()),
				workloadmetafx.Module(defaults.DefaultParams()),
				autodiscoveryimpl.Module(),
			)
		},
	}

	// Add flag for core config (optional)
	cmd.Flags().StringVarP(&cliParams.CoreConfigPath, "core-config", "C", defaultCoreConfigPath, "Path to the core configuration file (optional)")
	// Add flag for inactivity timeout (optional)
	cmd.Flags().IntVarP(&cliParams.inactivityTimeout, "inactivity-timeout", "t", 1, "Time (seconds) that the program will wait for new logs before exiting (optional)")

	return []*cobra.Command{cmd}
}

// runAnalyzeLogs initializes the launcher and sends the log config file path to the source provider.
func runAnalyzeLogs(cliParams *CliParams, config config.Component, ac autodiscovery.Component) error {
	outputChan, launchers, pipelineProvider := runAnalyzeLogsHelper(cliParams, config, ac)
	if outputChan == nil {
		return fmt.Errorf("Invalid input")
	}

	// Set up an inactivity timeout
	inactivityTimeout := time.Duration(cliParams.inactivityTimeout) * time.Second
	idleTimer := time.NewTimer(inactivityTimeout)

	for {
		select {
		case msg := <-outputChan:
			parsedMessage := processor.JSONPayload
			err := json.Unmarshal(msg.GetContent(), &parsedMessage)
			if err != nil {
				fmt.Printf("Failed to parse message: %v\n", err)
				continue
			}

			fmt.Println(parsedMessage.Message)

			// Reset the inactivity timer every time a message is processed
			if !idleTimer.Stop() {
				<-idleTimer.C
			}
			idleTimer.Reset(inactivityTimeout)
		case <-idleTimer.C:
			// Timeout reached, signal quit
			pipelineProvider.Stop()
			launchers.Stop()
			return nil
		}
	}
}

// Used to make testing easier
func runAnalyzeLogsHelper(cliParams *CliParams, config config.Component, ac autodiscovery.Component) (chan *message.Message, *launchers.Launchers, pipeline.Provider) {
	configSource := sources.NewConfigSources()
	waitTime := time.Duration(1) * time.Second
	waitCtx, cancelTimeout := context.WithTimeout(
		context.Background(), waitTime)
	common.LoadComponents(nil, nil, ac, pkgconfigsetup.Datadog().GetString("confd_path"))
	ac.LoadAndRun(context.Background())
	allConfigs, err := common.WaitForConfigsFromAD(waitCtx, []string{cliParams.LogConfigPath}, 1, "", ac)
	cancelTimeout()
	if err != nil {
		return nil, nil, nil
	}
	var sources []*sources.LogSource
	sources = nil
	for _, config := range allConfigs {
		if config.Name != cliParams.LogConfigPath {
			continue
		}
		sources, err = ad.CreateSources(integration.Config{
			Provider:   names.File,
			LogsConfig: config.LogsConfig,
		})
		if err != nil {
			fmt.Println("Cannot create source")
			return nil, nil, nil
		}
		break
	}

	if sources == nil {
		absolutePath := ""
		wd, err := os.Getwd()
		if err != nil {
			return nil, nil, nil
		}
		absolutePath = filepath.Join(wd, cliParams.LogConfigPath)
		data, err := os.ReadFile(absolutePath)
		if err != nil {
			fmt.Println("Cannot read file path of logs config")
			return nil, nil, nil
		}
		sources, err = ad.CreateSources(integration.Config{
			Provider:   names.File,
			LogsConfig: data,
		})
		if err != nil {
			fmt.Println("Cannot create source")
			return nil, nil, nil
		}
	}

	for _, source := range sources {
		if source.Config.TailingMode == "" {
			source.Config.TailingMode = "beginning"
		}
		configSource.AddSource(source)
	}
	return agentimpl.SetUpLaunchers(config, configSource)
}
