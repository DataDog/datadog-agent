// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package usm

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	sysconfigimpl "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cmdParams holds common CLI flags for USM commands.
type cmdParams struct {
	*command.GlobalParams
	outputJSON bool
}

// makeOneShotCommand creates a USM command that runs with FX OneShot pattern.
// This eliminates boilerplate for creating USM subcommands.
func makeOneShotCommand(
	globalParams *command.GlobalParams,
	use string,
	short string,
	runFunc interface{},
) *cobra.Command {
	params := &cmdParams{GlobalParams: globalParams}

	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(
				runFunc,
				fx.Supply(params),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(""),
					SysprobeConfigParams: sysconfigimpl.NewParams(
						sysconfigimpl.WithSysProbeConfFilePath(params.ConfFilePath),
						sysconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams: log.ForOneShot("SYS-PROBE", "off", false),
				}),
				core.Bundle(),
			)
		},
		SilenceUsage: true,
	}

	cmd.Flags().BoolVar(&params.outputJSON, "json", false, "Output as JSON")

	return cmd
}

// outputJSON prints data in JSON format with indentation.
// Note: This expects data to already be in a JSON-compatible format
// (e.g., parsed using yaml.v3 which produces map[string]interface{})
func outputJSON(data interface{}) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}
