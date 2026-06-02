// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package createschema implements 'agent createschema'.
package createschema

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"gopkg.in/yaml.v3"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/config/buildschema"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams
	Target string
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
	createSchemaCommand.Flags().StringVar(&cliParams.Target, "target", "", "schema to generate: core or system-probe")

	return []*cobra.Command{createSchemaCommand}
}

func run(cliParams *cliParams) error {
	// NOTE: Actual schema builder is done from an init() method in
	// the package pkg/config/setup. The code in pkg/config/create selects
	// the schemaBuilder when running this subcommand.

	var ddcfg model.Config
	if cliParams.Target == "core" {
		ddcfg = pkgconfigsetup.Datadog()
	} else if cliParams.Target == "system-probe" {
		ddcfg = pkgconfigsetup.SystemProbe()
	} else {
		return fmt.Errorf("unknown target '%s', valid ones are 'core' or 'system-probe'", cliParams.Target)
	}

	builder, ok := ddcfg.(buildschema.SchemaBuilder)
	if !ok {
		return errors.New("cannot use createschema without SchemaBuilder")
	}

	data, err := yaml.Marshal(builder.GetSchema())
	if err != nil {
		fmt.Printf("error: %s", err.Error())
		return err
	}
	fmt.Print(string(data))
	return nil
}
