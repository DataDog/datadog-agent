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
	"github.com/DataDog/datadog-agent/pkg/diagnose/connectivity"
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
func run(agentCfg config.Component, diagnoseComponent diagnose.Component, client ipc.HTTPClient, cliParams *cliParams) error {
	cfg := diagnose.Config{Include: cliParams.include}

	// Try to contact the running cluster-agent first so the live health-platform store is included.
	result, err := requestDiagnosesFromRunningAgent(agentCfg, client, cfg)
	if err != nil {
		// Agent not running — fall back to running connectivity suites in-process.
		// Health-platform issues are not available without a live store.
		fmt.Fprintf(color.Error, "Warning: cluster-agent not reachable (%v); running connectivity suites locally (health-platform issues unavailable).\n", err)
		result, err = runLocalSuites(diagnoseComponent, cfg)
		if err != nil {
			return err
		}
	}
	return format.Text(color.Output, cfg, result)
}

// runLocalSuites runs the connectivity diagnose suites in-process, matching the
// original cluster-agent diagnose behaviour before IPC was introduced.
func runLocalSuites(diagnoseComponent diagnose.Component, cfg diagnose.Config) (*diagnose.Result, error) {
	catalog := diagnose.GetCatalog()
	catalog.Register(diagnose.AutodiscoveryConnectivity, func(_ diagnose.Config) []diagnose.Diagnosis {
		return connectivity.DiagnoseMetadataAutodiscoveryConnectivity()
	})
	catalog.Register(diagnose.CoreEndpointsConnectivity, func(_ diagnose.Config) []diagnose.Diagnosis {
		return connectivity.Diagnose(diagnose.Config{}, nil)
	})

	suites := diagnose.Suites{}
	if len(cfg.Include) == 0 {
		// Default: only run autodiscovery connectivity (original default behaviour).
		if fn, ok := catalog.Suites[diagnose.AutodiscoveryConnectivity]; ok {
			suites[diagnose.AutodiscoveryConnectivity] = fn
		}
	} else {
		for _, name := range cfg.Include {
			if fn, ok := catalog.Suites[name]; ok {
				suites[name] = fn
			}
		}
	}

	if len(suites) == 0 {
		return &diagnose.Result{
			Runs: []diagnose.Diagnoses{
				{
					Name: "Diagnose",
					Diagnoses: []diagnose.Diagnosis{
						{
							Status:    diagnose.DiagnosisFail,
							Name:      "Diagnose",
							Category:  "All",
							Diagnosis: "No diagnose suite were found",
						},
					},
				},
			},
		}, nil
	}

	return diagnoseComponent.RunLocalSuite(suites, cfg)
}

// requestDiagnosesFromRunningAgent POSTs to the running cluster-agent's /diagnose endpoint
// so that the live health-platform store (and any other registered suites) is included.
func requestDiagnosesFromRunningAgent(agentCfg config.Component, client ipc.HTTPClient, cfg diagnose.Config) (*diagnose.Result, error) {
	port := agentCfg.GetInt("cluster_agent.cmd_port")
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
