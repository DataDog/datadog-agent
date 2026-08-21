// Unless explicitly stated otherwise all files in this repository is licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remotecommand implements the 'agent remote' subcommand, which proxies CLI commands to
// registered remote agents through the Remote Agent Registry.
package remotecommand

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcfx "github.com/DataDog/datadog-agent/comp/core/ipc/fx"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// cliParams are the command-line arguments for this subcommand.
type cliParams struct {
	*command.GlobalParams

	jsonOutput bool
	verbose    bool
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	params := &cliParams{
		GlobalParams: globalParams,
	}

	remoteCmd := &cobra.Command{
		Use:          "remote",
		Short:        "Execute commands on registered remote agents",
		Long:         `The 'remote' subcommand proxies CLI commands to remote agents registered with the Core Agent through the Remote Agent Registry.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	remoteCmd.PersistentFlags().BoolVarP(&params.jsonOutput, "json", "j", false, "format output as JSON")
	remoteCmd.PersistentFlags().BoolVarP(&params.verbose, "verbose", "v", false, "verbose output")

	listCmd := &cobra.Command{
		Use:          "list",
		Short:        "List commands available on registered remote agents",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runListCommands(params)
		},
	}

	executeCmd := &cobra.Command{
		Use:          "execute <command_path> [args...]",
		Short:        "Execute a command on a registered remote agent",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				return fmt.Errorf("command_path is required")
			}
			return runExecuteCommand(params, args[0], args[1:])
		},
	}

	remoteCmd.AddCommand(listCmd, executeCmd)

	return []*cobra.Command{remoteCmd}
}

// runListCommands calls ListCommands on the Core Agent and prints the available commands.
func runListCommands(params *cliParams) error {
	return fxutil.OneShot(
		func(_ log.Component, _ config.Component, ipc ipc.Component) error {
			grpclog.SetLoggerV2(grpclog.NewLoggerV2(io.Discard, io.Discard, io.Discard))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conn, err := dialCoreAgent(ctx, ipc)
			if err != nil {
				return err
			}
			defer conn.Close()

			cli := pb.NewRemoteCommandProviderClient(conn)
			resp, err := cli.ListCommands(ctx, &pb.ListCommandsRequest{})
			if err != nil {
				return err
			}

			for _, cmd := range resp.GetCommands() {
				flavor := cmd.GetAgentFlavor()
				if flavor == "" {
					flavor = "unknown"
				}
				fmt.Printf("%s\t%s\t%s\n", flavor, cmd.GetName(), cmd.GetHelper())
			}
			return nil
		},
		fx.Supply(params),
		fx.Supply(command.GetDefaultCoreBundleParams(params.GlobalParams)),
		core.Bundle(),
		ipcfx.ModuleReadOnly(),
	)
}

// runExecuteCommand calls ExecuteCommand on the Core Agent to proxy a command to a remote agent.
func runExecuteCommand(params *cliParams, commandPath string, args []string) error {
	arguments := &structpb.Struct{Fields: make(map[string]*structpb.Value)}
	for _, arg := range args {
		// Forward positional args as a string list.
		arguments.Fields["args"] = structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue(arg)}})
	}

	return fxutil.OneShot(
		func(_ log.Component, _ config.Component, ipc ipc.Component) error {
			grpclog.SetLoggerV2(grpclog.NewLoggerV2(io.Discard, io.Discard, io.Discard))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			conn, err := dialCoreAgent(ctx, ipc)
			if err != nil {
				return err
			}
			defer conn.Close()

			cli := pb.NewRemoteCommandProviderClient(conn)
			resp, err := cli.ExecuteCommand(ctx, &pb.ExecuteCommandRequest{
				CommandPath: commandPath,
				Arguments:   arguments,
				JsonOutput:   params.jsonOutput,
				Verbose:      params.verbose,
			})
			if err != nil {
				return err
			}

			if resp.GetStdout() != "" {
				fmt.Print(resp.GetStdout())
			}
			if resp.GetStderr() != "" {
				fmt.Fprint(os.Stderr, resp.GetStderr())
			}
			if len(resp.GetBinaryOutput()) > 0 {
				os.Stdout.Write(resp.GetBinaryOutput())
			}
			if resp.GetExitCode() != 0 {
				os.Exit(int(resp.GetExitCode()))
			}
			return nil
		},
		fx.Supply(params),
		fx.Supply(command.GetDefaultCoreBundleParams(params.GlobalParams)),
		core.Bundle(),
		ipcfx.ModuleReadOnly(),
	)
}

// dialCoreAgent creates a gRPC connection to the Core Agent's IPC endpoint.
func dialCoreAgent(ctx context.Context, ipc ipc.Component) (*grpc.ClientConn, error) {
	md := metadata.MD{
		"authorization": []string{"Bearer " + ipc.GetAuthToken()},
	}
	ctx = metadata.NewOutgoingContext(ctx, md)

	return grpc.DialContext( //nolint:staticcheck // TODO (ASC) fix grpc.DialContext is deprecated
		ctx,
		fmt.Sprintf(":%v", pkgconfigsetup.Datadog().GetInt("cmd_port")),
		grpc.WithTransportCredentials(credentials.NewTLS(ipc.GetTLSClientConfig())),
	)
}
