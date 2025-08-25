// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package remoteconfig implements 'agent remote-config'.
package remoteconfig

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/flare"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	agentgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
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
	remoteConfigCmd := &cobra.Command{
		Use:   "remote-config",
		Short: "Remote configuration state command",
		Long:  ``,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(state,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot(command.LoggerName, "OFF", false),
				}),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
		Hidden: true,
	}

	remoteConfigCmd.AddCommand(
		&cobra.Command{
			Use:   "reset",
			Short: "Reset the remote configuration state",
			Long:  ``,
			RunE: func(_ *cobra.Command, _ []string) error {
				return fxutil.OneShot(reset,
					fx.Supply(cliParams),
					fx.Supply(core.BundleParams{
						ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
						LogParams:    log.ForOneShot(command.LoggerName, "OFF", false),
					}),
					core.Bundle(),
					ipcfx.ModuleReadOnly(),
				)
			},
			Hidden: true,
		},
	)

	return []*cobra.Command{remoteConfigCmd}
}

func reset(_ *cliParams, config config.Component, ipc ipc.Component) error {
	if !pkgconfigsetup.IsRemoteConfigEnabled(config) {
		return errors.New("remote configuration is not enabled")
	}
	fmt.Println("Resetting the remote configuration state...")

	ctx, closeFn := context.WithCancel(context.Background())
	defer closeFn()
	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", ipc.GetAuthToken())},
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return err
	}

	cli, err := agentgrpc.GetDDAgentSecureClient(ctx, ipcAddress, pkgconfigsetup.GetIPCPort(), ipc.GetTLSClientConfig())
	if err != nil {
		return err
	}
	in := new(emptypb.Empty)

	_, err = cli.ResetConfigState(ctx, in)
	if err != nil {
		return fmt.Errorf("couldn't get the repositories state: %w", err)
	}
	return nil
}

func state(_ *cliParams, config config.Component, ipc ipc.Component) error {
	if !pkgconfigsetup.IsRemoteConfigEnabled(config) {
		return errors.New("remote configuration is not enabled")
	}
	fmt.Println("Fetching the configuration and director repos state..")
	// Call GRPC endpoint returning state tree

	ctx, closeFn := context.WithCancel(context.Background())
	defer closeFn()
	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", ipc.GetAuthToken())}, // TODO IPC: use GRPC client
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return err
	}

	cli, err := agentgrpc.GetDDAgentSecureClient(ctx, ipcAddress, pkgconfigsetup.GetIPCPort(), ipc.GetTLSClientConfig())
	if err != nil {
		return err
	}
	in := new(emptypb.Empty)

	s, err := cli.GetConfigState(ctx, in)
	if err != nil {
		return fmt.Errorf("couldn't get the repositories state: %w", err)
	}

	var stateHA *pbgo.GetStateConfigResponse
	if pkgconfigsetup.Datadog().GetBool("multi_region_failover.enabled") {
		stateHA, err = cli.GetConfigStateHA(ctx, in)
		if err != nil {
			return fmt.Errorf("couldn't get the HA repositories state: %w", err)
		}
	}

	flare.PrintRemoteConfigStates(os.Stdout, s, stateHA)

	return nil
}
