// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package remotecommand implements the static 'agent remote' command and the dynamic command trees supplied by
// registered remote command providers.
package remotecommand

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"go.uber.org/fx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/grpclog"
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

// ExecuteFunc invokes a provider command with its public provider name, command path, and typed arguments.
type ExecuteFunc func(providerName string, commandPath []string, arguments *structpb.Struct, stdout, stderr io.Writer) error

type exitCodeError int

func (e exitCodeError) Error() string {
	return fmt.Sprintf("remote command exited with code %d", e)
}

func (e exitCodeError) ExitCode() int {
	return int(e)
}

var remoteGlobalParams sync.Map // map[*cobra.Command]*command.GlobalParams

// Commands returns the static remote parent. Command providers are attached before Cobra resolves their child names.
func Commands(globalParams *command.GlobalParams) []*cobra.Command {
	remote := &cobra.Command{
		Use:           "remote",
		Short:         "Run commands exposed by registered remote agents",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	remoteGlobalParams.Store(remote, globalParams)
	return []*cobra.Command{remote}
}

// Prepare discovers active command providers only when Cobra has resolved the static remote parent.
func Prepare(root *cobra.Command, args []string) error {
	remote, ok := remoteCommand(root, args)
	if !ok {
		return nil
	}
	globalParams, ok := remoteGlobalParams.Load(remote)
	if !ok {
		return errors.New("remote command was not initialized")
	}
	return fxutil.OneShot(func(_ log.Component, _ config.Component, ipc ipc.Component) error {
		grpclog.SetLoggerV2(grpclog.NewLoggerV2(io.Discard, io.Discard, io.Discard))
		ctx := context.Background()
		conn, err := dialCoreAgent(ipc)
		if err != nil {
			return err
		}
		defer conn.Close()
		client := pb.NewRemoteCommandProviderClient(conn)
		response, err := client.ListCommands(ctx, &pb.ListCommandsRequest{})
		if err != nil {
			return err
		}
		return AttachCommandProviders(remote, response.GetProviders(), func(providerName string, commandPath []string, arguments *structpb.Struct, stdout, stderr io.Writer) error {
			stream, err := client.ExecuteCommand(ctx, &pb.ExecuteCommandRequest{ProviderName: providerName, CommandPath: commandPath, Arguments: arguments})
			if err != nil {
				return err
			}
			for {
				frame, err := stream.Recv()
				if errors.Is(err, io.EOF) {
					return nil
				}
				if err != nil {
					return err
				}
				if err := renderFrame(frame, stdout, stderr); err != nil {
					return err
				}
			}
		})
	}, fx.Supply(command.GetDefaultCoreBundleParams(globalParams.(*command.GlobalParams))), core.Bundle(), ipcfx.ModuleReadOnly())
}

func remoteCommand(root *cobra.Command, args []string) (*cobra.Command, bool) {
	resolved, _, err := root.Find(args)
	return resolved, err == nil && resolved.Name() == "remote"
}

// AttachCommandProviders attaches one top-level Cobra command per active command provider.
func AttachCommandProviders(remote *cobra.Command, providers []*pb.CommandProvider, execute ExecuteFunc) error {
	if remote == nil {
		return errors.New("remote command is required")
	}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		if provider.GetName() == "" {
			return errors.New("command provider name is required")
		}
		providerCmd := &cobra.Command{
			Use:          provider.GetName(),
			Short:        provider.GetDescription(),
			SilenceUsage: true,
		}
		for _, providerCommand := range provider.GetCommands() {
			if err := addCommand(providerCmd, provider.GetName(), nil, providerCommand, execute); err != nil {
				return err
			}
		}
		remote.AddCommand(providerCmd)
	}
	return nil
}

func addCommand(parent *cobra.Command, providerName string, parentPath []string, definition *pb.Command, execute ExecuteFunc) error {
	if definition == nil {
		return nil
	}
	name := definition.GetShortName()
	if name == "" {
		name = definition.GetName()
		if index := strings.LastIndex(name, "."); index >= 0 {
			name = name[index+1:]
		}
	}
	if name == "" {
		return errors.New("command name is required")
	}
	cmd := &cobra.Command{
		Use:          name,
		Short:        definition.GetHelper(),
		Long:         definition.GetLongDescription(),
		SilenceUsage: true,
	}
	for _, parameter := range definition.GetParameters() {
		if parameter.GetIsFlag() {
			if err := addFlag(cmd, parameter); err != nil {
				return err
			}
		}
	}
	commandPath := append(append([]string{}, parentPath...), name)
	if definition.GetIsRunnable() {
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			arguments, err := argumentsForCommand(cmd, args, definition.GetParameters())
			if err != nil {
				return err
			}
			return execute(providerName, commandPath, arguments, cmd.OutOrStdout(), cmd.ErrOrStderr())
		}
	}
	for _, child := range definition.GetChildren() {
		if err := addCommand(cmd, providerName, commandPath, child, execute); err != nil {
			return err
		}
	}
	parent.AddCommand(cmd)
	return nil
}

func renderFrame(frame *pb.ExecuteCommandResponse, stdout, stderr io.Writer) error {
	if frame == nil {
		return errors.New("remote command returned no frame")
	}
	switch value := frame.GetFrame().(type) {
	case *pb.ExecuteCommandResponse_Stdout:
		_, err := io.WriteString(stdout, value.Stdout)
		return err
	case *pb.ExecuteCommandResponse_Stderr:
		_, err := io.WriteString(stderr, value.Stderr)
		return err
	case *pb.ExecuteCommandResponse_BinaryOutput:
		_, err := stdout.Write(value.BinaryOutput)
		return err
	case *pb.ExecuteCommandResponse_ExitCode:
		if value.ExitCode != 0 {
			return exitCodeError(value.ExitCode)
		}
		return nil
	default:
		return errors.New("remote command returned an empty frame")
	}
}

func addFlag(cmd *cobra.Command, parameter *pb.CommandParameter) error {
	name, shorthand, usage := parameter.GetName(), parameter.GetShortName(), parameter.GetHelper()
	switch parameter.GetType() {
	case pb.ParameterType_TYPE_INT:
		cmd.Flags().IntP(name, shorthand, 0, usage)
	case pb.ParameterType_TYPE_UINT:
		cmd.Flags().UintP(name, shorthand, 0, usage)
	case pb.ParameterType_TYPE_FLOAT:
		cmd.Flags().Float64P(name, shorthand, 0, usage)
	case pb.ParameterType_TYPE_BOOL:
		cmd.Flags().BoolP(name, shorthand, false, usage)
	case pb.ParameterType_TYPE_STRING_SLICE:
		cmd.Flags().StringSliceP(name, shorthand, nil, usage)
	case pb.ParameterType_TYPE_INT_SLICE:
		cmd.Flags().IntSliceP(name, shorthand, nil, usage)
	case pb.ParameterType_TYPE_UINT_SLICE:
		cmd.Flags().UintSliceP(name, shorthand, nil, usage)
	case pb.ParameterType_TYPE_FLOAT_SLICE:
		cmd.Flags().Float64SliceP(name, shorthand, nil, usage)
	case pb.ParameterType_TYPE_STRING:
		cmd.Flags().StringP(name, shorthand, "", usage)
	default:
		return fmt.Errorf("unsupported parameter type %s", parameter.GetType())
	}
	if parameter.GetRequired() {
		_ = cmd.MarkFlagRequired(name)
	}
	return nil
}

func argumentsForCommand(cmd *cobra.Command, args []string, parameters []*pb.CommandParameter) (*structpb.Struct, error) {
	fields := map[string]*structpb.Value{}
	position := 0
	for _, parameter := range parameters {
		if parameter.GetIsFlag() {
			if !cmd.Flags().Changed(parameter.GetName()) {
				continue
			}
			value, err := flagValue(cmd, parameter)
			if err != nil {
				return nil, err
			}
			fields[parameter.GetName()] = value
			continue
		}
		if position >= len(args) {
			if parameter.GetRequired() {
				return nil, fmt.Errorf("required argument %q is missing", parameter.GetName())
			}
			continue
		}
		value, err := stringValue(parameter.GetType(), args[position])
		if err != nil {
			return nil, fmt.Errorf("invalid argument %q: %w", parameter.GetName(), err)
		}
		fields[parameter.GetName()] = value
		position++
	}
	if position != len(args) {
		return nil, fmt.Errorf("unexpected arguments: %s", strings.Join(args[position:], " "))
	}
	return &structpb.Struct{Fields: fields}, nil
}

func flagValue(cmd *cobra.Command, parameter *pb.CommandParameter) (*structpb.Value, error) {
	name := parameter.GetName()
	switch parameter.GetType() {
	case pb.ParameterType_TYPE_INT:
		v, err := cmd.Flags().GetInt(name)
		return structpb.NewNumberValue(float64(v)), err
	case pb.ParameterType_TYPE_UINT:
		v, err := cmd.Flags().GetUint(name)
		return structpb.NewNumberValue(float64(v)), err
	case pb.ParameterType_TYPE_FLOAT:
		v, err := cmd.Flags().GetFloat64(name)
		return structpb.NewNumberValue(v), err
	case pb.ParameterType_TYPE_BOOL:
		v, err := cmd.Flags().GetBool(name)
		return structpb.NewBoolValue(v), err
	case pb.ParameterType_TYPE_STRING_SLICE:
		v, err := cmd.Flags().GetStringSlice(name)
		return stringListValue(v), err
	case pb.ParameterType_TYPE_INT_SLICE:
		v, err := cmd.Flags().GetIntSlice(name)
		return numberListValue(v), err
	case pb.ParameterType_TYPE_UINT_SLICE:
		v, err := cmd.Flags().GetUintSlice(name)
		return uintListValue(v), err
	case pb.ParameterType_TYPE_FLOAT_SLICE:
		v, err := cmd.Flags().GetFloat64Slice(name)
		return floatListValue(v), err
	case pb.ParameterType_TYPE_STRING:
		v, err := cmd.Flags().GetString(name)
		return structpb.NewStringValue(v), err
	default:
		return nil, fmt.Errorf("unsupported parameter type %s", parameter.GetType())
	}
}

func stringValue(typ pb.ParameterType, raw string) (*structpb.Value, error) {
	switch typ {
	case pb.ParameterType_TYPE_INT:
		v, err := strconv.ParseInt(raw, 10, 64)
		return structpb.NewNumberValue(float64(v)), err
	case pb.ParameterType_TYPE_UINT:
		v, err := strconv.ParseUint(raw, 10, 64)
		return structpb.NewNumberValue(float64(v)), err
	case pb.ParameterType_TYPE_FLOAT:
		v, err := strconv.ParseFloat(raw, 64)
		return structpb.NewNumberValue(v), err
	case pb.ParameterType_TYPE_BOOL:
		v, err := strconv.ParseBool(raw)
		return structpb.NewBoolValue(v), err
	case pb.ParameterType_TYPE_STRING:
		return structpb.NewStringValue(raw), nil
	default:
		return nil, fmt.Errorf("unsupported parameter type %s", typ)
	}
}

func stringListValue(values []string) *structpb.Value {
	values2 := make([]*structpb.Value, len(values))
	for i, v := range values {
		values2[i] = structpb.NewStringValue(v)
	}
	return structpb.NewListValue(&structpb.ListValue{Values: values2})
}
func numberListValue(values []int) *structpb.Value {
	values2 := make([]*structpb.Value, len(values))
	for i, v := range values {
		values2[i] = structpb.NewNumberValue(float64(v))
	}
	return structpb.NewListValue(&structpb.ListValue{Values: values2})
}
func uintListValue(values []uint) *structpb.Value {
	values2 := make([]*structpb.Value, len(values))
	for i, v := range values {
		values2[i] = structpb.NewNumberValue(float64(v))
	}
	return structpb.NewListValue(&structpb.ListValue{Values: values2})
}
func floatListValue(values []float64) *structpb.Value {
	values2 := make([]*structpb.Value, len(values))
	for i, v := range values {
		values2[i] = structpb.NewNumberValue(v)
	}
	return structpb.NewListValue(&structpb.ListValue{Values: values2})
}

func dialCoreAgent(ipc ipc.Component) (*grpc.ClientConn, error) {
	return grpc.NewClient(fmt.Sprintf(":%v", pkgconfigsetup.Datadog().GetInt("cmd_port")), grpc.WithTransportCredentials(credentials.NewTLS(ipc.GetTLSClientConfig())))
}
