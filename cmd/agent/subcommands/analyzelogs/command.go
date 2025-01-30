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
	"time"

	"go.uber.org/fx"

	"github.com/spf13/cobra"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer/demultiplexerimpl"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/autodiscoveryimpl"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/names"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	dualTaggerfx "github.com/DataDog/datadog-agent/comp/core/tagger/fx-dual"
	wmcatalog "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/catalog"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/defaults"
	workloadmetafx "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx"
	"github.com/DataDog/datadog-agent/comp/forwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/logs/agent/agentimpl"
	"github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/inventorychecksimpl"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx"
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
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				dualTaggerfx.Module(common.DualTaggerParams()),
				wmcatalog.GetCatalog(),
				workloadmetafx.Module(defaults.DefaultParams()),
				autodiscoveryimpl.Module(),
				fx.Supply(context.Background()),
				forwarder.Bundle(defaultforwarder.NewParams(defaultforwarder.WithNoopForwarder())),
				inventorychecksimpl.Module(),
				logscompression.Module(),
				demultiplexerimpl.Module(demultiplexerimpl.NewDefaultParams(demultiplexerimpl.WithFlushInterval(0))),
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
func runAnalyzeLogs(cliParams *CliParams, config config.Component, ac autodiscovery.Component, wmeta workloadmeta.Component, secretResolver secrets.Component) error {
	outputChan, launchers, pipelineProvider := runAnalyzeLogsHelper(cliParams, config, ac, wmeta, secretResolver)
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
func runAnalyzeLogsHelper(cliParams *CliParams, config config.Component, ac autodiscovery.Component, wmeta workloadmeta.Component, secretResolver secrets.Component) (chan *message.Message, *launchers.Launchers, pipeline.Provider) {
	configSource := sources.NewConfigSources()
	fmt.Println("HOLY AIDS?")
	waitTime := time.Duration(cliParams.inactivityTimeout) * time.Second
	waitCtx, _ := context.WithTimeout(
		context.Background(), waitTime)
	fmt.Println("analyze logs ANDREWQIAN1", ac)
	fmt.Println("analyze logs ANDREWQIAN2", pkgconfigsetup.Datadog().GetString("confd_path"))
	fmt.Println("analyze logs ANDREWQIAN3", context.Background())
	common.LoadComponents(nil, nil, ac, pkgconfigsetup.Datadog().GetString("confd_path"))
	ac.LoadAndRun(context.Background())
	fmt.Println("TEST TODAY ANALYZE LOGS-----")
	fmt.Println("hehexd1", waitCtx)
	fmt.Println("hehexd2", []string{cliParams.LogConfigPath})
	fmt.Println("hehexd3", 1)
	fmt.Println("hehexd4", "")
	fmt.Println("hehexd5", ac)
	allConfigs, err := common.WaitForConfigsFromAD(waitCtx, []string{cliParams.LogConfigPath}, 1, "", ac)
	// cancelTimeout()
	if err != nil {
		return nil, nil, nil
	}
	var sources []*sources.LogSource
	sources = nil
	fmt.Println("AGENT ANALYZE LOGS ALL CONFIGS ", allConfigs)
	for _, config := range allConfigs {
		if config.Name != cliParams.LogConfigPath {
			continue
		}
		fmt.Println("CONFIG IS ???", config)
		fmt.Println("CONFIG Instances IS ???", config.Instances)
		fmt.Println("CONFIG LogsConfig IS ???", config.LogsConfig)
		sources, err = ad.CreateSources(integration.Config{
			Provider:   names.File,
			LogsConfig: config.Instances[0],
		})
		fmt.Println("SOURCE ERROR?", err)
		fmt.Println("SOURCES IS???", sources)
		break
	}

	if sources == nil {
		absolutePath := ""
		wd, err := os.Getwd()
		if err != nil {
			fmt.Println("Cannot get working directory")
			return nil, nil, nil
		}
		absolutePath = wd + "/" + cliParams.LogConfigPath

		data, err := os.ReadFile(absolutePath)
		fmt.Println("HEHEXD", data)
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
