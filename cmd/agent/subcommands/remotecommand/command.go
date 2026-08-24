// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remotecommand implements the 'agent remote' subcommand, which proxies CLI commands to
// registered remote agents through the Remote Agent Registry.
package remotecommand

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
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

type cliParams struct {
	*command.GlobalParams
	jsonOutput    bool
	verbose       bool
	agentID       string
	argumentsJSON string
}

func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	params := &cliParams{GlobalParams: globalParams}
	remoteCmd := &cobra.Command{Use: "remote", Short: "Execute commands on registered remote agents", SilenceUsage: true, RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}
	remoteCmd.PersistentFlags().BoolVarP(&params.jsonOutput, "json", "j", false, "format output as JSON")
	remoteCmd.PersistentFlags().BoolVarP(&params.verbose, "verbose", "v", false, "verbose output")
	listCmd := &cobra.Command{Use: "list", Short: "List commands available on registered remote agents", SilenceUsage: true, Args: cobra.NoArgs, RunE: func(_ *cobra.Command, _ []string) error { return runListCommands(params) }}
	executeCmd := &cobra.Command{Use: "execute <command_path>", Short: "Execute a command on a registered remote agent", SilenceUsage: true, Args: cobra.ExactArgs(1), RunE: func(_ *cobra.Command, args []string) error { return runExecuteCommand(params, args[0]) }}
	executeCmd.Flags().StringVar(&params.agentID, "agent-id", "", "opaque agent ID from remote list")
	executeCmd.Flags().StringVar(&params.argumentsJSON, "arguments", "{}", "JSON object of named command arguments")
	_ = executeCmd.MarkFlagRequired("agent-id")
	remoteCmd.AddCommand(listCmd, executeCmd)
	return []*cobra.Command{remoteCmd}
}

func runListCommands(params *cliParams) error {
	return fxutil.OneShot(func(_ log.Component, _ config.Component, ipc ipc.Component) error {
		grpclog.SetLoggerV2(grpclog.NewLoggerV2(io.Discard, io.Discard, io.Discard))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		conn, err := dialCoreAgent(ctx, ipc)
		if err != nil {
			return err
		}
		defer conn.Close()
		resp, err := pb.NewRemoteCommandProviderClient(conn).ListCommands(ctx, &pb.ListCommandsRequest{})
		if err != nil {
			return err
		}
		if params.jsonOutput {
			return json.NewEncoder(os.Stdout).Encode(resp.GetCommands())
		}
		for _, cmd := range flattenCommands(resp.GetCommands()) {
			fmt.Printf("%s\t%s\t%s\t%s\n", cmd.GetAgentId(), cmd.GetAgentFlavor(), cmd.GetName(), cmd.GetHelper())
		}
		return nil
	}, fx.Supply(params), fx.Supply(command.GetDefaultCoreBundleParams(params.GlobalParams)), core.Bundle(), ipcfx.ModuleReadOnly())
}

func runExecuteCommand(params *cliParams, commandPath string) error {
	arguments, err := parseArguments(params.argumentsJSON)
	if err != nil {
		return err
	}
	return fxutil.OneShot(func(_ log.Component, _ config.Component, ipc ipc.Component) error {
		grpclog.SetLoggerV2(grpclog.NewLoggerV2(io.Discard, io.Discard, io.Discard))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		conn, err := dialCoreAgent(ctx, ipc)
		if err != nil {
			return err
		}
		defer conn.Close()
		cli := pb.NewRemoteCommandProviderClient(conn)
		listed, err := cli.ListCommands(ctx, &pb.ListCommandsRequest{})
		if err != nil {
			return err
		}
		cmd := findCommand(listed.GetCommands(), params.agentID, commandPath)
		if cmd == nil {
			return fmt.Errorf("command %q was not found for agent %q", commandPath, params.agentID)
		}
		if !cmd.GetIsRunnable() {
			return fmt.Errorf("command %q is not executable", commandPath)
		}
		if err := validateArguments(cmd, arguments); err != nil {
			return err
		}
		resp, err := cli.ExecuteCommand(ctx, &pb.ExecuteCommandRequest{CommandPath: commandPath, AgentId: params.agentID, Arguments: arguments, JsonOutput: params.jsonOutput, Verbose: params.verbose})
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
			_, _ = os.Stdout.Write(resp.GetBinaryOutput())
		}
		if resp.GetExitCode() != 0 {
			os.Exit(int(resp.GetExitCode()))
		}
		return nil
	}, fx.Supply(params), fx.Supply(command.GetDefaultCoreBundleParams(params.GlobalParams)), core.Bundle(), ipcfx.ModuleReadOnly())
}

func parseArguments(raw string) (*structpb.Struct, error) {
	var values map[string]any
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	if values == nil {
		return nil, fmt.Errorf("arguments must be a JSON object")
	}
	return structpb.NewStruct(values)
}
func flattenCommands(commands []*pb.Command) []*pb.Command {
	var flattened []*pb.Command
	for _, command := range commands {
		if command == nil {
			continue
		}
		flattened = append(flattened, command)
		flattened = append(flattened, flattenCommands(command.GetChildren())...)
	}
	return flattened
}

func findCommand(commands []*pb.Command, agentID, path string) *pb.Command {
	return findCommandWithPersistentParameters(commands, agentID, path, nil)
}

func findCommandWithPersistentParameters(commands []*pb.Command, agentID, path string, inherited []*pb.CommandParameter) *pb.Command {
	for _, command := range commands {
		if command == nil {
			continue
		}
		parameters := append(append([]*pb.CommandParameter{}, inherited...), command.GetParameters()...)
		if command.GetAgentId() == agentID && command.GetName() == path {
			copy := *command
			copy.Parameters = parameters
			return &copy
		}
		persistent := append([]*pb.CommandParameter{}, inherited...)
		for _, parameter := range command.GetParameters() {
			if parameter.GetIsPersistent() {
				persistent = append(persistent, parameter)
			}
		}
		if found := findCommandWithPersistentParameters(command.GetChildren(), agentID, path, persistent); found != nil {
			return found
		}
	}
	return nil
}
func validateArguments(command *pb.Command, arguments *structpb.Struct) error {
	parameters := map[string]*pb.CommandParameter{}
	for _, parameter := range command.GetParameters() {
		parameters[parameter.GetName()] = parameter
	}
	for name, value := range arguments.GetFields() {
		parameter, ok := parameters[name]
		if !ok {
			return fmt.Errorf("unknown argument %q", name)
		}
		if !matchesType(parameter.GetType(), value) {
			return fmt.Errorf("argument %q has the wrong type", name)
		}
	}
	for _, parameter := range parameters {
		if parameter.GetRequired() && arguments.GetFields()[parameter.GetName()] == nil {
			return fmt.Errorf("required argument %q is missing", parameter.GetName())
		}
	}
	return nil
}
func matchesType(typ pb.ParameterType, value *structpb.Value) bool {
	if value == nil {
		return false
	}
	list := value.GetListValue()
	scalar := func(t pb.ParameterType, v *structpb.Value) bool {
		switch t {
		case pb.ParameterType_TYPE_STRING:
			_, ok := v.Kind.(*structpb.Value_StringValue)
			return ok
		case pb.ParameterType_TYPE_BOOL:
			_, ok := v.Kind.(*structpb.Value_BoolValue)
			return ok
		case pb.ParameterType_TYPE_FLOAT:
			_, ok := v.Kind.(*structpb.Value_NumberValue)
			return ok
		case pb.ParameterType_TYPE_INT:
			n, ok := v.Kind.(*structpb.Value_NumberValue)
			return ok && math.Trunc(n.NumberValue) == n.NumberValue
		case pb.ParameterType_TYPE_UINT:
			n, ok := v.Kind.(*structpb.Value_NumberValue)
			return ok && n.NumberValue >= 0 && math.Trunc(n.NumberValue) == n.NumberValue
		default:
			return false
		}
	}
	switch typ {
	case pb.ParameterType_TYPE_STRING_SLICE, pb.ParameterType_TYPE_INT_SLICE, pb.ParameterType_TYPE_UINT_SLICE, pb.ParameterType_TYPE_FLOAT_SLICE:
		if list == nil {
			return false
		}
		base := map[pb.ParameterType]pb.ParameterType{pb.ParameterType_TYPE_STRING_SLICE: pb.ParameterType_TYPE_STRING, pb.ParameterType_TYPE_INT_SLICE: pb.ParameterType_TYPE_INT, pb.ParameterType_TYPE_UINT_SLICE: pb.ParameterType_TYPE_UINT, pb.ParameterType_TYPE_FLOAT_SLICE: pb.ParameterType_TYPE_FLOAT}[typ]
		for _, item := range list.GetValues() {
			if !scalar(base, item) {
				return false
			}
		}
		return true
	default:
		return scalar(typ, value)
	}
}
func dialCoreAgent(ctx context.Context, ipc ipc.Component) (*grpc.ClientConn, error) {
	ctx = metadata.NewOutgoingContext(ctx, metadata.MD{"authorization": []string{"Bearer " + ipc.GetAuthToken()}})
	return grpc.DialContext(ctx, fmt.Sprintf(":%v", pkgconfigsetup.Datadog().GetInt("cmd_port")), grpc.WithTransportCredentials(credentials.NewTLS(ipc.GetTLSClientConfig())))
}
