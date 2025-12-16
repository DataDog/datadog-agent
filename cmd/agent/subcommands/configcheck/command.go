// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configcheck implements 'agent configcheck'.
package configcheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

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
	secretfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx"
	"github.com/DataDog/datadog-agent/pkg/flare"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const unknownProvider = "Unknown provider"
const unknownConfigSource = "Unknown configuration source"

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
				secretfx.Module(),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}
	configCheckCommand.Flags().BoolVarP(&cliParams.verbose, "verbose", "v", false, "print additional debug info")
	configCheckCommand.Flags().BoolVarP(&cliParams.json, "json", "j", false, "print out raw json")
	configCheckCommand.Flags().BoolVarP(&cliParams.prettyJSON, "pretty-json", "p", false, "pretty print json (takes priority over --json)")

	return []*cobra.Command{configCheckCommand}
}

func run(cliParams *cliParams, log log.Component, client ipc.HTTPClient) error {
	// no specific check was passed, print all check configs
	if len(cliParams.args) < 1 {
		return fullConfigCmd(cliParams, log, client)
	}

	// print only the config of the specified check
	return singleCheckCmd(cliParams, log, client)
}

func fullConfigCmd(cliParams *cliParams, _ log.Component, client ipc.HTTPClient) error {
	cr, err := getConfigCheckResponse(client)
	if err != nil {
		return err
	}

	var b bytes.Buffer
	color.Output = &b

	if cliParams.json || cliParams.prettyJSON {
		checkConfigs := make([]checkConfig, len(cr.Configs))

		// gather and filter every check config
		for i, config := range cr.Configs {
			checkConfigs[i] = convertCheckConfigToJSON(config.Config, config.InstanceIDs)
		}

		if err := printJSON(color.Output, checkConfigs, cliParams.prettyJSON); err != nil {
			return err
		}

	} else {
		flare.PrintConfigCheck(color.Output, cr, cliParams.verbose)
	}

	fmt.Println(b.String())
	return nil
}

func singleCheckCmd(cliParams *cliParams, _ log.Component, client ipc.HTTPClient) error {
	if len(cliParams.args) > 1 {
		return errors.New("only one check must be specified")
	}

	cr, err := getConfigCheckResponse(client)
	if err != nil {
		return err
	}

	// search through the configs for a check with the same name
	for _, configResponse := range cr.Configs {
		if cliParams.args[0] == configResponse.Config.Name {
			var b bytes.Buffer
			color.Output = &b

			if cliParams.json || cliParams.prettyJSON {
				checkConfig := convertCheckConfigToJSON(configResponse.Config, configResponse.InstanceIDs)

				if err := printJSON(color.Output, checkConfig, cliParams.prettyJSON); err != nil {
					return err
				}

			} else {
				// flare format print
				flare.PrintConfigWithInstanceIDs(color.Output, configResponse.Config, configResponse.InstanceIDs, "")
			}

			fmt.Println(b.String())
			return nil
		}
	}

	// return an error if the name wasn't found in the checks list
	return fmt.Errorf("no check named %q was found", cliParams.args[0])
}

func getConfigCheckResponse(client ipc.HTTPClient) (integration.ConfigCheckResponse, error) {
	var cr integration.ConfigCheckResponse

	endpoint, err := client.NewIPCEndpoint("/agent/config-check")
	if err != nil {
		return cr, err
	}

	res, err := endpoint.DoGet()
	if err != nil {
		return cr, fmt.Errorf("the agent ran into an error while checking config: %v", err)
	}

	err = json.Unmarshal(res, &cr)
	if err != nil {
		return cr, fmt.Errorf("unable to parse configcheck: %v", err)
	}

	return cr, nil
}

func convertCheckConfigToJSON(c integration.Config, instanceIDs []string) checkConfig {
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

func printJSON(w io.Writer, rawJSON any, prettyPrintJSON bool) error {
	var result []byte
	var err error

	// convert to bytes and indent
	if prettyPrintJSON {
		result, err = json.MarshalIndent(rawJSON, "", "  ")
	} else {
		result, err = json.Marshal(rawJSON)
	}
	if err != nil {
		return err
	}

	fmt.Fprint(w, string(result))
	return nil
}
