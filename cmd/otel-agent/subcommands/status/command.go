// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package status implements the core status component information provider interface
package status

import (
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	delegatedauthfx "github.com/DataDog/datadog-agent/comp/core/delegatedauth/fx"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logfx "github.com/DataDog/datadog-agent/comp/core/log/fx"
	secretsnoopfx "github.com/DataDog/datadog-agent/comp/core/secrets/fx-noop"
	status "github.com/DataDog/datadog-agent/comp/otelcol/status/def"
	otelagentStatusfx "github.com/DataDog/datadog-agent/comp/otelcol/status/fx"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In

	Status status.Component
}

const headerText = "==========\nOTel Agent\n==========\n"

// MakeCommand returns a `status` command to be used by agent binaries.
func MakeCommand(globalConfGetter func() *subcommands.GlobalParams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print the current status",
		Long:  ``,
		RunE: func(*cobra.Command, []string) error {
			globalParams := globalConfGetter()
			return fxutil.OneShot(
				runStatus,
				fx.Supply(coreconfig.NewAgentParams(globalParams.CoreConfPath, coreconfig.WithExtraConfFiles(globalParams.ConfPaths))),
				secretsnoopfx.Module(), // TODO: secret-enabled: is this required ?
				fx.Supply(log.ForOneShot(globalParams.LoggerName, "off", true)),
				coreconfig.Module(),
				logfx.Module(),
				ipcfx.ModuleReadOnly(),
				otelagentStatusfx.Module(),
				delegatedauthfx.Module(),
			)
		},
	}

	return cmd
}

func runStatus(deps dependencies) error {
	statusText, err := deps.Status.GetStatus()
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write([]byte(headerText + statusText))
	if err != nil {
		return err
	}
	return nil
}
