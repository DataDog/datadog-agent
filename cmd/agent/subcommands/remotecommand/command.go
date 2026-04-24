// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remotecommand implements 'agent remote' for managing remote agent commands.
package remotecommand

import (
	"context"
	"fmt"
	"os"
	"strings"

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
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
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

	remoteCmd := &cobra.Command{
		Use:   "remote",
		Short: "Manage commands exposed by remote agents",
		Long:  `Interact with commands exposed by remote agents registered with the Core Agent.`,
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all available remote commands",
		Long:  `Lists all CLI commands exposed by remote agents that are currently registered with the Core Agent.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return fxutil.OneShot(listRemoteCommands,
				fx.Supply(cliParams),
				fx.Supply(core.BundleParams{
					ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
					LogParams:    log.ForOneShot(command.LoggerName, "off", true),
				}),
				core.Bundle(),
				ipcfx.ModuleReadOnly(),
			)
		},
	}

	remoteCmd.AddCommand(listCmd)

	return []*cobra.Command{remoteCmd}
}

func listRemoteCommands(_ log.Component, _ config.Component, ipcComp ipc.Component) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := newAgentSecureClient(ctx, ipcComp)
	if err != nil {
		return err
	}

	resp, err := client.ListRemoteCommands(ctx, &emptypb.Empty{})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not list remote commands. Is the agent running?")
		return err
	}

	if len(resp.AgentCommands) == 0 {
		fmt.Println("No remote commands available.")
		return nil
	}

	fmt.Println("Available remote commands:")
	fmt.Println()

	for i, group := range resp.AgentCommands {
		if i > 0 {
			fmt.Println()
		}
		fmt.Println(group.AgentName)
		fmt.Println(strings.Repeat("-", len(group.AgentName)))
		fmt.Println()
		printCommands(group.Commands, "  ")
	}

	return nil
}

// printCommands prints a tree of commands with indentation.
func printCommands(commands []*pb.Command, indent string) {
	for _, cmd := range commands {
		fmt.Printf("%s%s (%s)\n", indent, cmd.Name, cmd.Helper)
		if len(cmd.Children) > 0 {
			printCommands(cmd.Children, indent+"  ")
		}
	}
}

func newAgentSecureClient(ctx context.Context, ipcComp ipc.Component) (pb.AgentSecureClient, error) {
	md := metadata.MD{
		"authorization": []string{"Bearer " + ipcComp.GetAuthToken()},
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
	if err != nil {
		return nil, err
	}

	client, err := agentgrpc.GetDDAgentSecureClient(ctx, ipcAddress, pkgconfigsetup.GetIPCPort(), ipcComp.GetTLSClientConfig())
	if err != nil {
		return nil, err
	}

	return client, nil
}
