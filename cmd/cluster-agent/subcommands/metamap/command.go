// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && kubeapiserver

// Package metamap implements 'cluster-agent metamap'.
package metamap

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	ipchttp "github.com/DataDog/datadog-agent/comp/core/ipc/httphelpers"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/status/render"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type cliParams struct {
	args []string
}

// Commands returns a slice of subcommands for the 'cluster-agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{}

	cmd := &cobra.Command{
		Use:   "metamap [nodeName]",
		Short: "Print the map between the metadata and the pods associated",
		Long: `The metamap command is mostly designed for troubleshooting purposes.
One can easily identify which pods are running on which nodes,
as well as which services are serving the pods. Or the deployment name for the pod`,
		Example: "datadog-cluster-agent metamap ip-10-0-115-123",
		RunE: func(_ *cobra.Command, args []string) error {
			cliParams.args = args
			return fxutil.OneShot(run,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewClusterAgentParams(globalParams.ConfFilePath),
					SecretParams: secrets.NewEnabledParams(),
					LogParams:    log.ForOneShot(command.LoggerName, command.DefaultLogLevel, true),
				}),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	return []*cobra.Command{cmd}
}

//nolint:revive // TODO(CINT) Fix revive linter
func run(log log.Component, config config.Component, client ipc.HTTPClient, cliParams *cliParams) error {
	nodeName := ""
	if len(cliParams.args) > 0 {
		nodeName = cliParams.args[0]
	}
	return getMetadataMap(client, nodeName) // if nodeName == "", call all.
}

func getMetadataMap(client ipc.HTTPClient, nodeName string) error {
	var e error
	var urlstr string
	if nodeName == "" {
		urlstr = fmt.Sprintf("https://localhost:%v/api/v1/tags/pod", pkgconfigsetup.Datadog().GetInt("cluster_agent.cmd_port"))
	} else {
		urlstr = fmt.Sprintf("https://localhost:%v/api/v1/tags/pod/%s", pkgconfigsetup.Datadog().GetInt("cluster_agent.cmd_port"), nodeName)
	}

	r, e := client.Get(urlstr, ipchttp.WithLeaveConnectionOpen)
	if e != nil {
		fmt.Printf(`
		Could not reach agent: %v
		Make sure the agent is properly running before requesting the map of services to pods.
		Contact support if you continue having issues.`, e)
		return e
	}

	formattedMetadataMap, err := render.FormatMetadataMapCLI(r)
	if err != nil {
		return err
	}

	fmt.Println(formattedMetadataMap)

	return nil
}
