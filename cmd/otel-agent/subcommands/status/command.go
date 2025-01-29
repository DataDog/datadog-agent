// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package status implements the core status component information provider interface
package status

import (
	"context"
	agentConfig "github.com/DataDog/datadog-agent/cmd/otel-agent/config"
	"github.com/DataDog/datadog-agent/cmd/otel-agent/subcommands"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	status "github.com/DataDog/datadog-agent/comp/otelcol/status/def"
	otelagentStatusfx "github.com/DataDog/datadog-agent/comp/otelcol/status/fx"
	pkgconfigenv "github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"os"
)

type dependencies struct {
	fx.In

	Status status.Component
}

// MakeCommand returns a `version` command to be used by agent binaries.
func MakeCommand(globalConfGetter func() *subcommands.GlobalParams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print the current status",
		Long:  ``,
		RunE: func(*cobra.Command, []string) error {
			globalParams := globalConfGetter()
			acfg, err := agentConfig.NewConfigComponent(context.Background(), globalParams.CoreConfPath, globalParams.ConfPaths)
			if err != nil {
				return err
			}
			uris := append(globalParams.ConfPaths, globalParams.Sets...)
			return fxutil.OneShot(
				runStatus,
				fx.Supply(uris),
				fx.Provide(func() (coreconfig.Component, error) {
					pkgconfigenv.DetectFeatures(acfg)
					return acfg, nil
				}),
				otelagentStatusfx.Module(),
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
	_, err = os.Stdout.Write([]byte(statusText))
	if err != nil {
		return err
	}
	return nil
}
