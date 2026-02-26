// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package configcheck implements 'agent createschema'.
package createschema

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
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
		RunE: func(_ *cobra.Command, args []string) error {
			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(cliParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(cliParams.FleetPoliciesDirPath)),
				}),
				core.Bundle(),
			)
		},
	}

	return []*cobra.Command{createSchemaCommand}
}

func run(cliParams *cliParams) error {
	data, err := yaml.Marshal(pkgconfigsetup.Datadog().GetSchema())
	if err == nil {
		os.WriteFile("core_schema.yaml", data, 0644)
		fmt.Printf("Wrote core_schema.yaml\n")
	}
	data, err = yaml.Marshal(pkgconfigsetup.SystemProbe().GetSchema())
	if err == nil {
		os.WriteFile("system-probe_schema.yaml", data, 0644)
		fmt.Printf("Wrote system-probe_schema.yaml\n")
	}

	fmt.Printf("Exiting...\n")
	os.Exit(1)
	return nil
}
