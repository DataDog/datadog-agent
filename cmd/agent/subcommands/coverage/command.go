// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build e2ecoverage

package coverage

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	apiutil "github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams

	coverdir           string
	logLevelDefaultOff command.LogLevelDefaultOff
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
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot(command.LoggerName, cliParams.logLevelDefaultOff.Value(), false)}), // never output anything but hostname
				core.Bundle(),
			)
		},
	}
	c.AddCommand(generateCmd)
	cliParams.logLevelDefaultOff.Register(c)
	c.PersistentFlags().StringVar(&cliParams.coverdir, "coverdir", "", "Directory to store coverage files")
	return []*cobra.Command{c}
}

func requestCoverage(_ log.Component, config config.Component, params *cliParams) error {
	endpoint, err := apiutil.NewIPCEndpoint(config, "/agent/coverage")
	if err != nil {
		return err
	}
	v := url.Values{}
	if params.coverdir != "" {
		v.Add("coverdir", params.coverdir)
	}
	resp, err := endpoint.DoGet(apiutil.WithValues(v))
	if err != nil {
		return err
	}
	fmt.Printf("Coverage request sent, response: %s\n", resp)
	return nil
}
