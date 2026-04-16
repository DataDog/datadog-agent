// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remotecommand implements 'agent remote-commands' and the remote command execution fallback.
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
	agentgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

func init() {
	// Wire up the remote command handler so the root cobra command can
	// delegate unknown subcommands to remote agents.
	command.RemoteCommandHandler = TryRemoteCommand
}

// cliParams are the command-line arguments for this subcommand
type cliParams struct {
	*command.GlobalParams
}

// Commands returns a slice of subcommands for the 'agent' command.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	cliParams := &cliParams{
		GlobalParams: globalParams,
	}

	remoteCommandsCmd := &cobra.Command{
		Use:   "remote-commands",
		Short: "Manage remote agent commands",
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

	remoteCommandsCmd.AddCommand(listCmd)

	return []*cobra.Command{remoteCommandsCmd}
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

	if len(resp.Commands) == 0 {
		fmt.Println("No remote commands available.")
		return nil
	}

	fmt.Println("Available remote commands:")
	fmt.Println()
	printCommands(resp.Commands, "")

	return nil
}

// printCommands prints a tree of commands with indentation.
func printCommands(commands []*pb.Command, indent string) {
	for _, cmd := range commands {
		runnable := ""
		if cmd.RunE {
			runnable = " (runnable)"
		}
		fmt.Printf("%s%-20s %s%s\n", indent, cmd.Name, cmd.Helper, runnable)
		if len(cmd.Children) > 0 {
			printCommands(cmd.Children, indent+"  ")
		}
	}
}

// TryRemoteCommand attempts to execute a remote command via the Core Agent.
// This is called from the root cobra command's RunE when no static subcommand matches.
func TryRemoteCommand(globalParams *command.GlobalParams, args []string) error {
	return fxutil.OneShot(func(_ log.Component, _ config.Component, ipcComp ipc.Component) error {
		return executeRemoteCommand(ipcComp, args)
	},
		fx.Supply(core.BundleParams{
			ConfigParams: config.NewAgentParams(globalParams.ConfFilePath, config.WithExtraConfFiles(globalParams.ExtraConfFilePath), config.WithFleetPoliciesDirPath(globalParams.FleetPoliciesDirPath)),
			LogParams:    log.ForOneShot(command.LoggerName, "off", true),
		}),
		core.Bundle(),
		ipcfx.ModuleReadOnly(),
	)
}

func executeRemoteCommand(ipcComp ipc.Component, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client, err := newAgentSecureClient(ctx, ipcComp)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Could not connect to the Datadog Agent. Is the agent running?")
		return err
	}

	// Separate command path words from flags.
	// Everything before the first "--" prefixed arg is part of the command path.
	var commandParts []string
	var flagArgs []string
	hitFlags := false
	for _, arg := range args {
		if !hitFlags && strings.HasPrefix(arg, "-") {
			hitFlags = true
		}
		if hitFlags {
			flagArgs = append(flagArgs, arg)
		} else {
			commandParts = append(commandParts, arg)
		}
	}

	commandPath := strings.Join(commandParts, " ")

	// Build arguments struct from flag args (as simple key-value pairs)
	arguments, err := buildArguments(flagArgs)
	if err != nil {
		return fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Check for common flags
	jsonOutput := false
	verbose := false
	for _, arg := range flagArgs {
		if arg == "--json" || arg == "-j" {
			jsonOutput = true
		}
		if arg == "--verbose" || arg == "-v" {
			verbose = true
		}
	}

	req := &pb.ExecuteCommandRequest{
		CommandPath: commandPath,
		Arguments:   arguments,
		JsonOutput:  jsonOutput,
		Verbose:     verbose,
	}

	resp, err := client.ExecuteRemoteCommand(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to execute remote command %q: %v\n", commandPath, err)
		return err
	}

	if resp.Stdout != "" {
		fmt.Print(resp.Stdout)
	}
	if resp.Stderr != "" {
		fmt.Fprint(os.Stderr, resp.Stderr)
	}

	if resp.ExitCode != 0 {
		os.Exit(int(resp.ExitCode))
	}

	return nil
}

// buildArguments converts flag args (e.g., ["--verbose", "--count", "5"]) into a protobuf Struct.
func buildArguments(flagArgs []string) (*structpb.Struct, error) {
	if len(flagArgs) == 0 {
		return nil, nil
	}

	fields := make(map[string]interface{})
	for i := 0; i < len(flagArgs); i++ {
		arg := flagArgs[i]
		if !strings.HasPrefix(arg, "-") {
			// Positional arg, store with index
			fields[fmt.Sprintf("arg_%d", i)] = arg
			continue
		}

		// Strip leading dashes
		key := strings.TrimLeft(arg, "-")

		// Check if this is a boolean flag (no value follows) or a key-value flag
		if i+1 < len(flagArgs) && !strings.HasPrefix(flagArgs[i+1], "-") {
			fields[key] = flagArgs[i+1]
			i++ // skip the value
		} else {
			fields[key] = true
		}
	}

	return structpb.NewStruct(fields)
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
