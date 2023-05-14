// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package remoteconfig implements 'agent remote-config'.
package remoteconfig

import (
	"context"
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
	"github.com/DataDog/datadog-agent/pkg/flare"
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
					ConfigParams: config.NewAgentParamsWithoutSecrets(globalParams.ConfFilePath)}),
				core.Bundle,
			)
		},
		Hidden: true,
	}

	return []*cobra.Command{remoteConfigCmd}
}

func state(cliParams *cliParams, config config.Component) error {
	if !config.GetBool("remote_configuration.enabled") {
		return fmt.Errorf("Remote configuration is not enabled")
	}
	fmt.Println("Fetching the configuration and director repos state..")
	// Call GRPC endpoint returning state tree

	token, err := security.FetchAuthToken()
	if err != nil {
		return fmt.Errorf("Couldn't get auth token: %v", err)
	}
	ctx, close := context.WithCancel(context.Background())
	defer close()
	md := metadata.MD{
		"authorization": []string{fmt.Sprintf("Bearer %s", token)},
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	cli, err := agentgrpc.GetDDAgentSecureClient(ctx)
	if err != nil {
		return err
	}
	in := new(emptypb.Empty)

	s, err := cli.GetConfigState(ctx, in)
	if err != nil {
		return fmt.Errorf("Couldn't get the repositories state: %v", err)
	}

	flare.PrintRemoteConfigState(os.Stdout, s)

	return nil
}
