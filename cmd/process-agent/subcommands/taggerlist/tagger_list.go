// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package app

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/process-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/tagger/api"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

const taggerListURLTpl = "https://%s/agent/tagger-list"

// Commands returns a slice of subcommands for the `tagger-list` command in the Process Agent
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	taggerCmd := &cobra.Command{
		Use:   "tagger-list",
		Short: "Print the tagger content of a running agent",
		Long:  "",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(taggerList,
				fx.Supply(command.GetCoreBundleParamsForOneShot(globalParams)),

				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
		SilenceUsage: true,
	}

	return []*cobra.Command{taggerCmd}
}

type dependencies struct {
	fx.In

	Config config.Component
	Log    log.Component
	Client ipc.HTTPClient
}

func taggerList(deps dependencies) error {
	deps.Log.Info("Got a request for the tagger-list. Calling tagger.")

	taggerURL, err := getTaggerURL()
	if err != nil {
		return err
	}

	return api.GetTaggerList(deps.Client, color.Output, taggerURL)
}

func getTaggerURL() (string, error) {
	addressPort, err := pkgconfigsetup.GetProcessAPIAddressPort(pkgconfigsetup.Datadog())
	if err != nil {
		return "", fmt.Errorf("config error: %s", err.Error())
	}
	return fmt.Sprintf(taggerListURLTpl, addressPort), nil
}
