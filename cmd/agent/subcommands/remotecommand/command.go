// Unless explicitly stated otherwise all files in this repository are licensed
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
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
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

// subagentNameMapping provides an optional mapping from a remote agent's flavor to the subcommand name
// used in the CLI. This avoids stuttering: e.g., the "agent_data_plane" flavor maps to "data-plane"
// so the user runs "agent data-plane dogstatsd top" instead of "agent agent-data-plane dogstatsd top".
//
// If a flavor is not in this map, the sanitized flavor is used as the subcommand name.
var subagentNameMapping = map[string]string{
	"agent_data_plane": "data-plane",
}

// cliParams are the command-line arguments for this subcommand.
type cliParams struct {
	*command.GlobalParams

	jsonOutput bool
	verbose    bool
}

// Commands returns a slice of subcommands for the 'agent' command.
//
// The root "remote" command is a grouping command that dynamically discovers subcommands at runtime
// by querying the Core Agent's gRPC server for commands registered by remote agents.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	remoteCmd := &cobra.Command{
		Use:   "remote",
		Short: "Execute commands on registered remote agents",
		Long: `The 'remote' subcommand proxies CLI commands to remote agents registered with the Core Agent
through the Remote Agent Registry. Available subcommands are discovered dynamically at runtime.`,
		SilenceUsage: true,
		// This command is not directly runnable; it serves as a grouping node.
		// When invoked without a subcommand, it prints help.
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	remoteCmd.PersistentFlags().BoolVarP(&cliParams.jsonOutput, "json", "j", false, "format output as JSON")
	remoteCmd.PersistentFlags().BoolVarP(&cliParams.verbose, "verbose", "v", false, "verbose output")

	// We build the dynamic subcommand tree at execution time, not at command construction time.
	// This is because the subcommands depend on the running Core Agent's state.
	//
	// We use PersistentPreRunE on the root "remote" command to query the Core Agent and populate
	// child commands before cobra dispatches to the actual subcommand.
	remoteCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		// Only build the dynamic tree if we haven't already.
		if cmd.HasSubCommands() {
			return nil
		}

		commands, err := fetchRemoteCommands(cliParams)
		if err != nil {
			// If we can't reach the agent, print a helpful message but don't fail the help.
			fmt.Fprintf(os.Stderr, "Warning: could not fetch remote commands from the agent: %v\n", err)
			return nil
		}

		buildCommandTree(cmd, commands, cliParams)
		return nil
	}

	return []*cobra.Command{remoteCmd}
}

// fetchRemoteCommands connects to the Core Agent's gRPC server and calls ListCommands to discover
// all commands exposed by registered remote agents.
func fetchRemoteCommands(params *cliParams) ([]*pb.Command, error) {
	// We use fxutil.OneShot to load config and IPC auth, then call the gRPC ListCommands RPC.
	var commands []*pb.Command

	err := fxutil.OneShot(
		func(_ log.Component, config config.Component, ipc ipc.Component) error {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			md := metadata.MD{
				"authorization": []string{"Bearer " + ipc.GetAuthToken()},
			}
			ctx = metadata.NewOutgoingContext(ctx, md)

			conn, err := grpc.DialContext( //nolint:staticcheck // TODO (ASC) fix grpc.DialContext is deprecated
				ctx,
				fmt.Sprintf(":%v", pkgconfigsetup.Datadog().GetInt("cmd_port")),
				grpc.WithTransportCredentials(credentials.NewTLS(ipc.GetTLSClientConfig())),
			)
			if err != nil {
				return err
			}
			defer conn.Close()

			cli := pb.NewRemoteCommandProviderClient(conn)
			resp, err := cli.ListCommands(ctx, &pb.ListCommandsRequest{})
			if err != nil {
				return err
			}

			commands = resp.GetCommands()
			return nil
		},
		fx.Supply(params),
		fx.Supply(command.GetDefaultCoreBundleParams(params.GlobalParams)),
		core.Bundle(),
		ipcfx.ModuleReadOnly(),
	)

	return commands, err
}

// buildCommandTree recursively builds cobra commands from the protobuf command tree.
func buildCommandTree(parent *cobra.Command, commands []*pb.Command, params *cliParams) {
	for _, cmd := range commands {
		cobraCmd := buildCobraCommand(cmd, params)
		parent.AddCommand(cobraCmd)
	}
}

// buildCobraCommand converts a protobuf Command into a cobra.Command, recursively building child commands.
func buildCobraCommand(pbCmd *pb.Command, params *cliParams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   pbCmd.GetName(),
		Short: pbCmd.GetHelper(),
		Long:  pbCmd.GetLongDescription(),
		// Silence usage errors since these are dynamic commands.
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	// Register flags from the command's parameters.
	for _, param := range pbCmd.GetParameters() {
		registerFlag(cmd, param)
	}

	if pbCmd.GetIsRunnable() {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			return executeRemoteCommand(c, params, pbCmd)
		}
	} else {
		// Non-runnable commands serve as grouping nodes.
		cmd.RunE = func(c *cobra.Command, args []string) error {
			return c.Help()
		}
	}

	// Recursively build child commands.
	for _, child := range pbCmd.GetChildren() {
		childCmd := buildCobraCommand(child, params)
		cmd.AddCommand(childCmd)
	}

	return cmd
}

// registerFlag adds a cobra flag from a protobuf CommandParameter.
func registerFlag(cmd *cobra.Command, param *pb.CommandParameter) {
	name := "--" + param.GetName()
	if param.GetShortName() != "" {
		// cobra expects single-character short names without the dash.
	}

	switch param.GetType() {
	case pb.ParameterType_TYPE_BOOL:
		var val bool
		if param.GetShortName() != "" {
			cmd.Flags().BoolVarP(&val, param.GetName(), param.GetShortName(), false, param.GetHelper())
		} else {
			cmd.Flags().BoolVar(&val, param.GetName(), false, param.GetHelper())
		}
	case pb.ParameterType_TYPE_INT:
		var val int
		if param.GetShortName() != "" {
			cmd.Flags().IntVarP(&val, param.GetName(), param.GetShortName(), 0, param.GetHelper())
		} else {
			cmd.Flags().IntVar(&val, param.GetName(), 0, param.GetHelper())
		}
	case pb.ParameterType_TYPE_UINT:
		var val uint
		if param.GetShortName() != "" {
			cmd.Flags().UintVarP(&val, param.GetName(), param.GetShortName(), 0, param.GetHelper())
		} else {
			cmd.Flags().UintVar(&val, param.GetName(), 0, param.GetHelper())
		}
	case pb.ParameterType_TYPE_FLOAT:
		var val float64
		if param.GetShortName() != "" {
			cmd.Flags().Float64VarP(&val, param.GetName(), param.GetShortName(), 0, param.GetHelper())
		} else {
			cmd.Flags().Float64Var(&val, param.GetName(), 0, param.GetHelper())
		}
	case pb.ParameterType_TYPE_STRING_SLICE:
		var val []string
		if param.GetShortName() != "" {
			cmd.Flags().StringSliceVarP(&val, param.GetName(), param.GetShortName(), nil, param.GetHelper())
		} else {
			cmd.Flags().StringSliceVar(&val, param.GetName(), nil, param.GetHelper())
		}
	case pb.ParameterType_TYPE_INT_SLICE:
		var val []int
		if param.GetShortName() != "" {
			cmd.Flags().IntSliceVarP(&val, param.GetName(), param.GetShortName(), nil, param.GetHelper())
		} else {
			cmd.Flags().IntSliceVar(&val, param.GetName(), nil, param.GetHelper())
		}
	default:
		// Default to string for TYPE_STRING and TYPE_UNSPECIFIED.
		var val string
		if param.GetShortName() != "" {
			cmd.Flags().StringVarP(&val, param.GetName(), param.GetShortName(), "", param.GetHelper())
		} else {
			cmd.Flags().StringVar(&val, param.GetName(), "", param.GetHelper())
		}
	}
	_ = name // name is used for documentation; the flag is registered above.
}

// executeRemoteCommand collects the flag values from the cobra command, constructs an ExecuteCommandRequest,
// and calls the Core Agent's gRPC server to proxy the command to the remote agent.
func executeRemoteCommand(cmd *cobra.Command, params *cliParams, pbCmd *pb.Command) error {
	// Build the command path by walking up the cobra command tree.
	commandPath := buildCommandPath(cmd)

	// Collect flag values into a protobuf Struct.
	arguments := &structpb.Struct{Fields: make(map[string]*structpb.Value)}
	collectFlagValues(cmd, pbCmd, arguments)

	// Also walk up to collect persistent flags from parent commands.
	parent := cmd.Parent()
	for parent != nil {
		parentPbCmd := findCommandByName(parent.Name(), pbCmd)
		if parentPbCmd != nil {
			collectFlagValues(parent, parentPbCmd, arguments)
		}
		parent = parent.Parent()
	}

	return fxutil.OneShot(
		func(_ log.Component, _ config.Component, ipc ipc.Component) error {
			// shut up grpc client!
			grpclog.SetLoggerV2(grpclog.NewLoggerV2(io.Discard, io.Discard, io.Discard))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			md := metadata.MD{
				"authorization": []string{"Bearer " + ipc.GetAuthToken()},
			}
			ctx = metadata.NewOutgoingContext(ctx, md)

			conn, err := grpc.DialContext( //nolint:staticcheck // TODO (ASC) fix grpc.DialContext is deprecated
				ctx,
				fmt.Sprintf(":%v", pkgconfigsetup.Datadog().GetInt("cmd_port")),
				grpc.WithTransportCredentials(credentials.NewTLS(ipc.GetTLSClientConfig())),
			)
			if err != nil {
				return err
			}
			defer conn.Close()

			cli := pb.NewRemoteCommandProviderClient(conn)
			resp, err := cli.ExecuteCommand(ctx, &pb.ExecuteCommandRequest{
				CommandPath: commandPath,
				Arguments:   arguments,
				JsonOutput:  params.jsonOutput,
				Verbose:     params.verbose,
			})
			if err != nil {
				return err
			}

			// Write stdout and stderr to the user's terminal.
			if resp.GetStdout() != "" {
				fmt.Print(resp.GetStdout())
			}
			if resp.GetStderr() != "" {
				fmt.Fprint(os.Stderr, resp.GetStderr())
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

// buildCommandPath constructs the full command path (e.g., "dogstatsd top") by walking up the cobra
// command tree from the given command to the "remote" root.
func buildCommandPath(cmd *cobra.Command) string {
	var parts []string
	for c := cmd; c != nil; c = c.Parent() {
		name := c.Name()
		if name == "remote" || name == filepath.Base(os.Args[0]) {
			break
		}
		parts = append([]string{name}, parts...)
	}
	return strings.Join(parts, " ")
}

// collectFlagValues reads flag values from the cobra command and writes them into the protobuf Struct.
func collectFlagValues(cmd *cobra.Command, pbCmd *pb.Command, arguments *structpb.Struct) {
	for _, param := range pbCmd.GetParameters() {
		if !param.GetIsFlag() {
			continue
		}
		flag := cmd.Flags().Lookup(param.GetName())
		if flag == nil {
			continue
		}
		if !flag.Changed {
			continue
		}
		arguments.Fields[param.GetName()] = flagValueToStructValue(flag, param)
	}
}

// flagValueToStructValue converts a cobra flag value to a protobuf Value based on the parameter type.
func flagValueToStructValue(flag *pflag.Flag, param *pb.CommandParameter) *structpb.Value {
	switch param.GetType() {
	case pb.ParameterType_TYPE_BOOL:
		return structpb.NewBoolValue(flag.Value.String() == "true")
	case pb.ParameterType_TYPE_INT:
		i, _ := strconv.ParseInt(flag.Value.String(), 10, 64)
		return structpb.NewNumberValue(float64(i))
	case pb.ParameterType_TYPE_UINT:
		u, _ := strconv.ParseUint(flag.Value.String(), 10, 64)
		return structpb.NewNumberValue(float64(u))
	case pb.ParameterType_TYPE_FLOAT:
		f, _ := strconv.ParseFloat(flag.Value.String(), 64)
		return structpb.NewNumberValue(f)
	case pb.ParameterType_TYPE_STRING_SLICE:
		return structpb.NewStringValue(flag.Value.String())
	case pb.ParameterType_TYPE_INT_SLICE:
		return structpb.NewStringValue(flag.Value.String())
	default:
		return structpb.NewStringValue(flag.Value.String())
	}
}

// findCommandByName searches for a command with the given name in the protobuf command tree.
// This is a simple helper used to find parent command metadata when collecting persistent flags.
func findCommandByName(name string, pbCmd *pb.Command) *pb.Command {
	if pbCmd.GetName() == name {
		return pbCmd
	}
	for _, child := range pbCmd.GetChildren() {
		if found := findCommandByName(name, child); found != nil {
			return found
		}
	}
	return nil
}
