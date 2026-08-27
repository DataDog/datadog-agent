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
	"github.com/spf13/pflag"
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

type providerDiscovery func(remote *cobra.Command, params *command.GlobalParams, args []string) error

// Prepare attaches provider commands before Cobra resolves an `agent remote` invocation, avoiding discovery for other commands.
func Prepare(root *cobra.Command, args []string) error {
	return prepare(root, args, discoverProviders)
}

func prepare(root *cobra.Command, args []string, discover providerDiscovery) error {
	remote, ok := remoteCommand(root, args)
	if !ok {
		return nil
	}
	globalParams, ok := remoteGlobalParams.Load(remote)
	if !ok {
		return errors.New("remote command was not initialized")
	}
	params := globalParams.(*command.GlobalParams)
	if err := applyGlobalFlags(root, args); err != nil {
		return err
	}
	return discover(remote, params, args)
}

func discoverProviders(remote *cobra.Command, globalParams *command.GlobalParams, args []string) error {
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
			if isBareRemoteInvocation(args) {
				setDiscoveryErrorMessage(remote, err)
				return nil
			}
			return err
		}
		if len(response.GetProviders()) == 0 {
			setEmptyProviderMessage(remote)
			return nil
		}
		return AttachCommandProviders(remote, response.GetProviders(), func(providerName string, commandPath []string, arguments *structpb.Struct, stdout, stderr io.Writer) error {
			return executeProviderCommand(globalParams, providerName, commandPath, arguments, stdout, stderr)
		})
	}, fx.Supply(command.GetDefaultCoreBundleParams(globalParams)), core.Bundle(), ipcfx.ModuleReadOnly())
}

// applyGlobalFlags applies root persistent flags through Cobra's authoritative flag set without consuming provider flags.
func applyGlobalFlags(root *cobra.Command, args []string) error {
	flags := root.PersistentFlags()
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" || !strings.HasPrefix(arg, "-") {
			continue
		}

		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		flag := flags.Lookup(name)
		if flag == nil {
			continue
		}
		if !hasValue {
			if flag.NoOptDefVal != "" {
				value = flag.NoOptDefVal
			} else if index+1 < len(args) {
				index++
				value = args[index]
			} else {
				return pflag.ErrHelp
			}
		}
		if err := flags.Set(flag.Name, value); err != nil {
			return err
		}
	}
	return nil
}

func executeProviderCommand(globalParams *command.GlobalParams, providerName string, commandPath []string, arguments *structpb.Struct, stdout, stderr io.Writer) error {
	return fxutil.OneShot(func(_ log.Component, _ config.Component, ipc ipc.Component) error {
		grpclog.SetLoggerV2(grpclog.NewLoggerV2(io.Discard, io.Discard, io.Discard))
		conn, err := dialCoreAgent(ipc)
		if err != nil {
			return err
		}
		defer conn.Close()

		stream, err := pb.NewRemoteCommandProviderClient(conn).ExecuteCommand(context.Background(), &pb.ExecuteCommandRequest{
			ProviderName: providerName,
			CommandPath:  commandPath,
			Arguments:    arguments,
		})
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
	}, fx.Supply(command.GetDefaultCoreBundleParams(globalParams)), core.Bundle(), ipcfx.ModuleReadOnly())
}

func isBareRemoteInvocation(args []string) bool {
	for index, arg := range args {
		if arg == "remote" {
			return index == len(args)-1
		}
	}
	return false
}

func setEmptyProviderMessage(remote *cobra.Command) {
	remote.RunE = func(cmd *cobra.Command, _ []string) error {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "No remote command providers are registered.")
		return err
	}
}

func setDiscoveryErrorMessage(remote *cobra.Command, discoveryErr error) {
	remote.RunE = func(cmd *cobra.Command, _ []string) error {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "Unable to discover remote command providers: %v\n", discoveryErr)
		return err
	}
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
	for _, providerGroup := range providers {
		if providerGroup == nil {
			continue
		}
		if providerGroup.GetName() == "" {
			return errors.New("command provider name is required")
		}
		providerCommand := &cobra.Command{
			Use:          providerGroup.GetName(),
			Short:        providerGroup.GetDescription(),
			SilenceUsage: true,
		}
		for _, commandDefinition := range providerGroup.GetCommands() {
			if err := addCommand(providerCommand, providerGroup.GetName(), nil, nil, commandDefinition, execute); err != nil {
				return err
			}
		}
		remote.AddCommand(providerCommand)
	}
	return nil
}

func addCommand(parent *cobra.Command, providerName string, parentPath []string, inheritedParameters []*pb.CommandParameter, definition *pb.Command, execute ExecuteFunc) error {
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
	parameters := append(append([]*pb.CommandParameter{}, inheritedParameters...), definition.GetParameters()...)
	inheritedPersistentParameters := append(append([]*pb.CommandParameter{}, inheritedParameters...), persistentParameters(definition.GetParameters())...)
	if definition.GetIsRunnable() {
		cmd.RunE = func(cmd *cobra.Command, args []string) error {
			arguments, err := argumentsForCommand(cmd, args, parameters)
			if err != nil {
				return err
			}
			return execute(providerName, commandPath, arguments, cmd.OutOrStdout(), cmd.ErrOrStderr())
		}
	}
	for _, child := range definition.GetChildren() {
		if err := addCommand(cmd, providerName, commandPath, inheritedPersistentParameters, child, execute); err != nil {
			return err
		}
	}
	parent.AddCommand(cmd)
	return nil
}

func persistentParameters(parameters []*pb.CommandParameter) []*pb.CommandParameter {
	persistent := make([]*pb.CommandParameter, 0, len(parameters))
	for _, parameter := range parameters {
		if parameter.GetIsPersistent() {
			persistent = append(persistent, parameter)
		}
	}
	return persistent
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
	flags := cmd.Flags()
	if parameter.GetIsPersistent() {
		flags = cmd.PersistentFlags()
	}
	switch parameter.GetType() {
	case pb.ParameterType_TYPE_INT:
		flags.IntP(name, shorthand, 0, usage)
	case pb.ParameterType_TYPE_UINT:
		flags.UintP(name, shorthand, 0, usage)
	case pb.ParameterType_TYPE_FLOAT:
		flags.Float64P(name, shorthand, 0, usage)
	case pb.ParameterType_TYPE_BOOL:
		flags.BoolP(name, shorthand, false, usage)
	case pb.ParameterType_TYPE_STRING_SLICE:
		flags.StringSliceP(name, shorthand, nil, usage)
	case pb.ParameterType_TYPE_INT_SLICE:
		flags.IntSliceP(name, shorthand, nil, usage)
	case pb.ParameterType_TYPE_UINT_SLICE:
		flags.UintSliceP(name, shorthand, nil, usage)
	case pb.ParameterType_TYPE_FLOAT_SLICE:
		flags.Float64SliceP(name, shorthand, nil, usage)
	case pb.ParameterType_TYPE_STRING:
		flags.StringP(name, shorthand, "", usage)
	default:
		return fmt.Errorf("unsupported parameter type %s", parameter.GetType())
	}
	if parameter.GetRequired() {
		if parameter.GetIsPersistent() {
			_ = cmd.MarkPersistentFlagRequired(name)
		} else {
			_ = cmd.MarkFlagRequired(name)
		}
	}
	return nil
}

func argumentsForCommand(cmd *cobra.Command, args []string, parameters []*pb.CommandParameter) (*structpb.Struct, error) {
	fields := map[string]*structpb.Value{}
	position := 0
	for parameterIndex, parameter := range parameters {
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
		if isSliceType(parameter.GetType()) {
			if hasFollowingPositionalParameter(parameters[parameterIndex+1:]) {
				return nil, fmt.Errorf("slice argument %q must be the final positional parameter", parameter.GetName())
			}
			value, err := positionalSliceValue(parameter.GetType(), args[position:])
			if err != nil {
				return nil, fmt.Errorf("invalid argument %q: %w", parameter.GetName(), err)
			}
			fields[parameter.GetName()] = value
			position = len(args)
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

func hasFollowingPositionalParameter(parameters []*pb.CommandParameter) bool {
	for _, parameter := range parameters {
		if !parameter.GetIsFlag() {
			return true
		}
	}
	return false
}

func isSliceType(typ pb.ParameterType) bool {
	return typ == pb.ParameterType_TYPE_STRING_SLICE ||
		typ == pb.ParameterType_TYPE_INT_SLICE ||
		typ == pb.ParameterType_TYPE_UINT_SLICE ||
		typ == pb.ParameterType_TYPE_FLOAT_SLICE
}

func positionalSliceValue(typ pb.ParameterType, args []string) (*structpb.Value, error) {
	elementType := map[pb.ParameterType]pb.ParameterType{
		pb.ParameterType_TYPE_STRING_SLICE: pb.ParameterType_TYPE_STRING,
		pb.ParameterType_TYPE_INT_SLICE:    pb.ParameterType_TYPE_INT,
		pb.ParameterType_TYPE_UINT_SLICE:   pb.ParameterType_TYPE_UINT,
		pb.ParameterType_TYPE_FLOAT_SLICE:  pb.ParameterType_TYPE_FLOAT,
	}[typ]

	values := make([]*structpb.Value, 0, len(args))
	for _, arg := range args {
		value, err := stringValue(elementType, arg)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return structpb.NewListValue(&structpb.ListValue{Values: values}), nil
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
