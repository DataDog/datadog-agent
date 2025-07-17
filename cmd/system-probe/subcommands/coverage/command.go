// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build e2ecoverage

// Package coverage implements 'system-probe coverage' useful to compute code coverage in E2E tests.
package coverage

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/system-probe/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/sysprobeconfigimpl"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams
}

// Commands initializes system-probe sub-command tree.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	c := &cobra.Command{
		Use:   "coverage",
		Short: "Handle running system-probe code coverage",
	}
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	generateCmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate running system-probe code coverage",
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(requestCoverage,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams:         config.NewAgentParams("", config.WithConfigMissingOK(true)),
					SysprobeConfigParams: sysprobeconfigimpl.NewParams(sysprobeconfigimpl.WithSysProbeConfFilePath(globalParams.ConfFilePath), sysprobeconfigimpl.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:            log.ForOneShot("SYS-PROBE", "off", false),
				}),
				core.Bundle(),
			)
		},
	}
	c.AddCommand(generateCmd)
	return []*cobra.Command{c}
}

func requestCoverage(_ log.Component, sysprobeconfig sysprobeconfig.Component, params *cliParams) error {
	cfg := sysprobeconfig.SysProbeObject()
	client := client.Get(cfg.SocketAddress)

	resp, err := client.Get("http://localhost/coverage")
	if err != nil {
		return err
	}
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()

	if err != nil {
		return err
	}
	fmt.Printf("Coverage request sent, response: %s\n", string(body))
	return nil
}
