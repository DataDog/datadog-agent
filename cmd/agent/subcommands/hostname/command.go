// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hostname implements 'agent hostname'.
package hostname

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}
	getHostnameCommand := &cobra.Command{
		Use:   "hostname",
		Short: "Print the hostname used by the Agent",
		Long:  ``,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(getHostname,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParamsWithoutSecrets(globalParams.ConfFilePath),
					LogParams:    log.LogForOneShot(command.LoggerName, "off", false)}), // never output anything but hostname
				core.Bundle,
			)
		},
	}

	return []*cobra.Command{getHostnameCommand}
}

func getHostname(log log.Component, config config.Component, cliParams *cliParams) error {
	hname, err := hostname.Get(context.TODO())
	if err != nil {
		return fmt.Errorf("Error getting the hostname: %v", err)
	}

	fmt.Println(hname)
	return nil
}
