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
	"github.com/DataDog/datadog-agent/pkg/api/security"
	pkgconfig "github.com/DataDog/datadog-agent/pkg/config"
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return fxutil.OneShot(state,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath)}),
				core.Bundle(),
			)
		},
		Hidden: true,
	}

	return []*cobra.Command{remoteConfigCmd}
}

func state(_ *cliParams, config config.Component) error {
	if !pkgconfig.IsRemoteConfigEnabled(config) {
		return errors.New("remote configuration is not enabled")
	}
	fmt.Println("Fetching the configuration and director repos state..")
	// Call GRPC endpoint returning state tree

	token, err := security.FetchAuthToken(config)
	if err != nil {
		return fmt.Errorf("couldn't get auth token: %w", err)
	}

	ctx, closeFn := context.WithCancel(context.Background())
	defer closeFn()
	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	ipcAddress, err := pkgconfig.GetIPCAddress()
	if err != nil {
		return err
	}

	cli, err := agentgrpc.GetDDAgentSecureClient(ctx, ipcAddress, pkgconfig.GetIPCPort())
	if err != nil {
		return err
	}
	in := new(emptypb.Empty)

	s, err := cli.GetConfigState(ctx, in)
	if err != nil {
		return fmt.Errorf("couldn't get the repositories state: %w", err)
	}

	var stateHA *pbgo.GetStateConfigResponse
	if pkgconfig.Datadog.GetBool("multi_region_failover.enabled") {
		stateHA, err = cli.GetConfigStateHA(ctx, in)
		if err != nil {
			return fmt.Errorf("couldn't get the HA repositories state: %w", err)
		}
	}

	flare.PrintRemoteConfigStates(os.Stdout, s, stateHA)

	return nil
}
