// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configcheck implements 'agent configcheck'.
package configcheck

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	jsonutil "github.com/DataDog/datadog-agent/pkg/util/json"
)

const (
	unknownProvider     = "Unknown provider"
	unknownConfigSource = "Unknown configuration source"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	// args are the positional command-line arguments
	args []string

	verbose    bool
	json       bool
	prettyJSON bool
}

type instance struct {
	ID     string `json:"id"`
	Config string `json:"config"`
}

type checkConfig struct {
	Name         string     `json:"check_name"`
	Provider     string     `json:"provider"`
	Source       string     `json:"source"`
	Instances    []instance `json:"instances"`
	InitConfig   string     `json:"init_config"`
	MetricConfig string     `json:"metric_config"`
	Logs         string     `json:"logs"`
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	configCheckCommand := &cobra.Command{
		Use:     "configcheck [check]",
		Aliases: []string{"checkconfig"},
		Short:   "Print all configurations loaded & resolved of a running agent",
		Long:    ``,
		RunE: func(_ *cobra.Command, args []string) error {
			cliParams.args = args

			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(cliParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(cliParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot("CORE", "off", true)}),
				core.Bundle(true),
				ipcfx.ModuleReadOnly(),
			)
		},
	}
	configCheckCommand.Flags().BoolVarP(&cliParams.verbose, "verbose", "v", false, "print additional debug info")
	configCheckCommand.Flags().BoolVarP(&cliParams.json, "json", "j", false, "print out raw json")
	configCheckCommand.Flags().BoolVarP(&cliParams.prettyJSON, "pretty-json", "p", false, "pretty print json (takes priority over --json)")

	return []*cobra.Command{configCheckCommand}
}

func run(cliParams *cliParams, _ log.Component, client ipc.HTTPClient) error {

	cr, err := getConfigCheckResponse(client)
	if err != nil {
		return err
	}

	// filter configs if a check name has been passed
	if len(cliParams.args) > 1 {
		return errors.New("only one check must be specified")
	} else if len(cliParams.args) == 1 {
		cr.Configs = findConfigsByName(cr.Configs, cliParams.args[0])
		if len(cr.Configs) == 0 {
			return fmt.Errorf("no check named %q was found", cliParams.args[0])
		}
	}

	if cliParams.json || cliParams.prettyJSON {
		// JSON formatted output
		checkJSONConfigs := make([]checkConfig, len(cr.Configs))
		for i, config := range cr.Configs {
			checkJSONConfigs[i] = convertCheckConfigToJSON(&config.Config, config.InstanceIDs)
		}

		if err := jsonutil.PrintJSON(color.Output, checkJSONConfigs, cliParams.prettyJSON, false, ""); err != nil {
			return err
		}
	} else {
		// flare-style formatted output
		flare.PrintConfigCheck(color.Output, *cr, cliParams.verbose)
	}

	return nil
}

func getConfigCheckResponse(client ipc.HTTPClient) (*integration.ConfigCheckResponse, error) {
	cr := &integration.ConfigCheckResponse{}

	endpoint, err := client.NewIPCEndpoint("/agent/config-check")
	if err != nil {
		return nil, err
	}

	res, err := endpoint.DoGet()
	if err != nil {
		return nil, fmt.Errorf("the agent ran into an error while checking config: %v", err)
	}

	err = json.Unmarshal(res, cr)
	if err != nil {
		return nil, fmt.Errorf("unable to parse configcheck: %v", err)
	}

	return cr, nil
}

func convertCheckConfigToJSON(c *integration.Config, instanceIDs []string) checkConfig {
	jsonConfig := checkConfig{}

	jsonConfig.Name = c.Name

	if c.Provider != "" {
		jsonConfig.Provider = c.Provider
	} else {
		jsonConfig.Provider = unknownProvider
	}

	if c.Source != "" {
		jsonConfig.Source = c.Source
	} else {
		jsonConfig.Source = unknownConfigSource
	}

	jsonConfig.Instances = make([]instance, len(c.Instances))
	for idx, content := range c.Instances {
		inst := instance{
			ID:     instanceIDs[idx],
			Config: string(content),
		}

		jsonConfig.Instances[idx] = inst
	}

	jsonConfig.InitConfig = string(c.InitConfig)
	jsonConfig.MetricConfig = string(c.MetricConfig)
	jsonConfig.Logs = string(c.LogsConfig)

	return jsonConfig
}

func findConfigsByName(configs []integration.ConfigResponse, name string) []integration.ConfigResponse {
	matchingConfigs := []integration.ConfigResponse{}

	for _, config := range configs {
		if config.Config.Name == name {
			matchingConfigs = append(matchingConfigs, config)
		}
	}

	return matchingConfigs
}
