// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package diagnose implements 'cluster-agent diagnose'.
package diagnose

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	diagnose "github.com/DataDog/datadog-agent/comp/core/diagnose/def"
	"github.com/DataDog/datadog-agent/comp/core/diagnose/format"
	diagnosefx "github.com/DataDog/datadog-agent/comp/core/diagnose/fx"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	include []string
}

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{}

	cmd := &cobra.Command{
		Use:   "diagnose",
		Short: "Execute some connectivity diagnosis on your system",
		Long:  ``,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath),
					LogParams:    log.ForOneShot(command.LoggerName, command.DefaultLogLevel, true),
				}),
				core.Bundle(core.WithSecrets()),
				diagnosefx.Module(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	cmd.Flags().StringSliceVar(&cliParams.include, "include", nil, "Comma-separated list of diagnosis to run")

	return []*cobra.Command{cmd}
}

//nolint:revive // TODO(CINT) Fix revive linter
func run(_ config.Component, _ diagnose.Component, client ipc.HTTPClient, cliParams *cliParams) error {
	cfg := diagnose.Config{Include: cliParams.include}
	result, err := requestDiagnosesFromRunningAgent(client, cfg)
	if err != nil {
		return fmt.Errorf("diagnose requires the cluster-agent to be running: %w", err)
	}
	return format.Text(color.Output, cfg, result)
}

// requestDiagnosesFromRunningAgent POSTs to the running cluster-agent's /diagnose endpoint
// so that the live health-platform store (and any other registered suites) is included.
func requestDiagnosesFromRunningAgent(client ipc.HTTPClient, cfg diagnose.Config) (*diagnose.Result, error) {
	port := pkgconfigsetup.Datadog().GetInt("cluster_agent.cmd_port")
	diagnoseURL := fmt.Sprintf("https://localhost:%d/diagnose", port)

	body, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("error serialising diagnose config: %w", err)
	}

	response, err := client.Post(diagnoseURL, "application/json", bytes.NewBuffer(body), ipchttp.WithCloseConnection)
	if err != nil {
		if response != nil && strings.TrimSpace(string(response)) != "" {
			return nil, fmt.Errorf("cluster-agent returned: %s", strings.TrimSpace(string(response)))
		}
		return nil, err
	}

	var result diagnose.Result
	if err := json.Unmarshal(response, &result); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}
	return &result, nil
}
