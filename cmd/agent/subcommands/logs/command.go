// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logs implements 'agent logs'.
package logs

import (
	"fmt"
	"net"
	"strconv"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type cliParams struct {
	*command.GlobalParams
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{GlobalParams: globalParams}

	clearHealthCmd := &cobra.Command{
		Use:   "clear-health",
		Short: "Clear the logs issues in Agent Health once their cause is fixed",
		Long: `Clear the "Logs Dropped Before Delivery" and "Log Data Lost After Rotation"
issues in Agent Health. Run it once you have fixed what made the Agent lose
logs: the issues resolve within 30 minutes, without restarting the Agent.
If the Agent is still losing logs, they come back.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(clearHealth,
				fx.Supply(cliParams),
				fx.Supply(command.GetDefaultCoreBundleParams(cliParams.GlobalParams)),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	logsCmd := &cobra.Command{
		Use:   "logs",
		Short: "Manage log collection in a running Agent",
	}
	logsCmd.AddCommand(clearHealthCmd)

	return []*cobra.Command{logsCmd}
}

func clearHealth(cfg config.Component, client ipc.HTTPClient, _ *cliParams) error {
	ipcAddress, err := pkgconfighelper.GetIPCAddress(cfg)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://%s/agent/logs/clear-health", net.JoinHostPort(ipcAddress, strconv.Itoa(cfg.GetInt("cmd_port"))))
	if _, err := client.Post(url, "application/json", nil); err != nil {
		// The endpoint exists only while the logs agent runs (logs_enabled: true).
		return fmt.Errorf("could not clear the logs issues; is log collection enabled and the Agent running? %w", err)
	}

	fmt.Println("Cleared the logs issues in Agent Health. They resolve within 30 minutes.")
	fmt.Println("If the Agent is still losing logs, they come back.")
	return nil
}
