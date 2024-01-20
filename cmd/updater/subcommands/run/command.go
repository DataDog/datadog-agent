// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package run implements 'updater run'.
package run

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/cmd/updater/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
	"github.com/DataDog/datadog-agent/pkg/updater"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"go.uber.org/fx"

	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"

	"github.com/spf13/cobra"
)

type cliParams struct {
	command.GlobalParams
}

// Commands returns the run command
func Commands(global *command.GlobalParams) []*cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Runs the updater",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFxWrapper(&cliParams{
				GlobalParams: *global,
			}, run)
		},
	}
	return []*cobra.Command{runCmd}
}

func runFxWrapper(params *cliParams, fct interface{}) error {
	return fxutil.Run(
		fx.Supply(params),
		fx.Supply(core.BundleParams{
			ConfigParams:         config.NewAgentParams(params.GlobalParams.ConfFilePath),
			SecretParams:         secrets.NewEnabledParams(),
			SysprobeConfigParams: sysprobeconfigimpl.NewParams(),
			LogParams:            logimpl.ForDaemon("UPDATER", "updater.log_file", pkgconfig.DefaultUpdaterLogFile),
		}),
		core.Bundle(),
		fx.Invoke(fct),
	)
}

func run(log log.Component, _ config.Component, params *cliParams) error {
	hostnameDetected, err := hostname.Get(context.TODO())
	if err != nil {
		return fmt.Errorf("could not get hostname: %w", err)
	}
	log.Infof("Hostname is: %s", hostnameDetected)
	configService, err := initRemoteConfig(hostnameDetected)
	if err != nil {
		return fmt.Errorf("could not init remote config: %w", err)
	}
	defer configService.Stop()
	orgConfig, err := updater.NewOrgConfig()
	if err != nil {
		return fmt.Errorf("could not create org config: %w", err)
	}
	u, err := updater.NewUpdater(orgConfig, params.Package)
	if err != nil {
		return fmt.Errorf("could not create updater: %w", err)
	}
	api, err := updater.NewLocalAPI(u)
	if err != nil {
		return fmt.Errorf("could not create local API: %w", err)
	}
	return api.Serve()
}

func initRemoteConfig(hostname string) (*service.Service, error) {
	configService, err := common.NewRemoteConfigService(hostname)
	if err != nil {
		return nil, fmt.Errorf("could not create remote config service: %w", err)
	}
	configService.Start(context.Background())
	return configService, nil
}
