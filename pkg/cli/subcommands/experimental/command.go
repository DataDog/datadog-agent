// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"fmt"
	"io"
	"path/filepath"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	configcmd "github.com/DataDog/datadog-agent/pkg/cli/subcommands/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/spf13/cobra"
)

// MakeCommand returns a hidden top-level `experimental` command with hidden
// subcommands. Neither the parent nor its children appear in any --help output;
// they are intentionally undocumented pending a feature flag decision.
func MakeCommand(globalParamsGetter func() configcmd.GlobalParams) *cobra.Command {
	ep := &experimentalParams{}

	experimentalCmd := &cobra.Command{
		Use:   "experimental",
		Short: "Experimental agent features",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	runE := func(cmd *cobra.Command, args []string) error {
		// Redirect the root command's error writer to discard. The agent's
		// runcmd.Run wrapper calls displayError(err, cmd.ErrOrStderr()) after
		// Execute() returns, which would print "Error: ..." to stderr and
		// duplicate our [ERR] formatted output. Since all errors are already
		// printed with [ERR]/[WARN] before being returned, we suppress the
		// secondary print by discarding that stream.
		cmd.Root().SetErr(io.Discard)

		globalParams := globalParamsGetter()
		ep.args = args
		ep.GlobalParams = globalParams

		// Pre-validate YAML syntax BEFORE fxutil.OneShot loads the config via
		// Viper. Without this, a tab-indented or malformed YAML produces a cryptic
		// Viper loader error ("unable to load Datadog config file: ...") before our
		// friendly checkYAMLSyntax ever runs.
		if globalParams.ConfFilePath != "" {
			// configName is always non-empty here; the thin adapter enforces
			// command.ConfigName = "datadog" before calling MakeCommand.
			configFile := filepath.Join(globalParams.ConfFilePath, globalParams.ConfigName+".yaml")
			if ok, warnings, err := checkYAMLSyntax(configFile); !ok {
				for _, w := range warnings {
					fmt.Printf("[WARN] yaml_syntax: %s\n", w)
				}
				fmt.Printf("[ERR]  yaml_syntax: %s\n", err)
				return err
			}
		}

		return fxutil.OneShot(runExperimentalCheck,
			fx.Supply(ep),
			fx.Supply(core.BundleParams{
				ConfigParams: config.NewAgentParams(
					globalParams.ConfFilePath,
					config.WithConfigName(globalParams.ConfigName),
					config.WithExtraConfFiles(globalParams.ExtraConfFilePaths),
					config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath),
				),
				LogParams: log.ForOneShot(globalParams.LoggerName, "off", true),
			}),
			core.Bundle(),
		)
	}

	for _, name := range []string{"check-config"} {
		sub := &cobra.Command{Use: name, Hidden: true, RunE: runE}
		sub.Flags().BoolVar(&ep.noAPICheck, "no-api", false, "")
		experimentalCmd.AddCommand(sub)
	}

	return experimentalCmd
}

type experimentalParams struct {
	configcmd.GlobalParams
	noAPICheck bool
	args       []string
}

func runExperimentalCheck(logger log.Component, cfg config.Component, p *experimentalParams) error {
	return runConfigCheck(logger, cfg, p.noAPICheck)
}
