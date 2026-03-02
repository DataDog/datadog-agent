// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package createschema implements 'agent createschema'.
package createschema

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"go.yaml.in/yaml/v2"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	createSchemaCommand := &cobra.Command{
		Use:     "createschema",
		Aliases: []string{"createschema"},
		Short:   "",
		Long:    ``,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(
						globalParams.ConfFilePath,
						config.WithExtraConfFiles(cliParams.ExtraConfFilePath),
						config.WithFleetPoliciesDirPath(cliParams.FleetPoliciesDirPath),
					),
				}),
				core.Bundle(),
			)
		},
	}

	return []*cobra.Command{createSchemaCommand}
}

func run(cfg config.Component, sysprobeConf sysprobeconfig.Component) error {
	data, err := yaml.Marshal(cfg.GetSchema())
	if err == nil {
		_ = os.WriteFile("core_schema.yaml", data, 0644)
		fmt.Printf("Wrote core_schema.yaml\n")
	}
	data, err = yaml.Marshal(sysprobeConf.GetSchema())
	if err == nil {
		_ = os.WriteFile("system-probe_schema.yaml", data, 0644)
		fmt.Printf("Wrote system-probe_schema.yaml\n")
	}

	fmt.Printf("Exiting...\n")
	os.Exit(0)
	return nil
}
