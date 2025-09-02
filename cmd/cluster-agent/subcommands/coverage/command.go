// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build e2ecoverage && !windows && kubeapiserver

// Package coverage implements 'cluster-agent coverage' useful to compute code coverage in E2E tests.
package coverage

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams
}

// Commands initializes dogstatsd sub-command tree.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	c := &cobra.Command{
		Use:   "coverage",
		Short: "Handle running agent code coverage",
	}
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate running agent code coverage",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(requestCoverage,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath),
					LogParams:    log.ForOneShot(command.LoggerName, command.DefaultLogLevel, true)}), // never output anything but hostname
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}
	c.AddCommand(generateCmd)
	return []*cobra.Command{c}
}

func requestCoverage(_ log.Component, config config.Component, ipc ipc.Component, params *cliParams) error {
	url := url.URL{
		Scheme: "https",
		Host:   fmt.Sprintf("localhost:%v", pkgconfigsetup.Datadog().GetInt("cluster_agent.cmd_port")),
		Path:   "/status",
	}
	resp, err := ipc.GetClient().Get(url.String())
	if err != nil {
		return err
	}

	fmt.Printf("Coverage request sent, response: %s\n", resp)
	return nil
}
