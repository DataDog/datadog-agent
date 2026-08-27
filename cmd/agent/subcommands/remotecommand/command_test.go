// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotecommand

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRemoteParentDisplaysHelp(t *testing.T) {
	remote := Commands(&command.GlobalParams{})[0]
	require.Equal(t, "remote", remote.Name())
	require.Empty(t, remote.Commands())
}

func TestBareRemoteInvocation(t *testing.T) {
	require.True(t, isBareRemoteInvocation([]string{"remote"}))
	require.True(t, isBareRemoteInvocation([]string{"--cfgpath", "custom.yaml", "remote"}))
	require.False(t, isBareRemoteInvocation([]string{"remote", "fixture-agent"}))
}

func TestDiscoveryErrorMessage(t *testing.T) {
	remote := Commands(&command.GlobalParams{})[0]
	var output bytes.Buffer
	remote.SetOut(&output)
	setDiscoveryErrorMessage(remote, errors.New("unavailable"))

	require.NoError(t, remote.Execute())
	require.Equal(t, "Unable to discover remote command providers: unavailable\n", output.String())
}

func TestEmptyProviderMessage(t *testing.T) {
	remote := Commands(&command.GlobalParams{})[0]
	var output bytes.Buffer
	remote.SetOut(&output)
	setEmptyProviderMessage(remote)

	require.NoError(t, remote.Execute())
	require.Equal(t, "No remote command providers are registered.\n", output.String())
}

func TestPrepareFxDependencies(t *testing.T) {
	root := &cobra.Command{Use: "agent"}
	root.AddCommand(Commands(&command.GlobalParams{})...)

	fxutil.TestOneShot(t, func() {
		require.NoError(t, Prepare(root, []string{"remote"}))
	})
}

func TestExecuteProviderCommandFxDependencies(t *testing.T) {
	fxutil.TestOneShot(t, func() {
		require.NoError(t, executeProviderCommand(&command.GlobalParams{}, "fixture-agent", []string{"diagnostics", "inspect"}, &structpb.Struct{}, io.Discard, io.Discard))
	})
}

func TestPrepareDiscoversRemoteProvidersBeforeCobraDispatch(t *testing.T) {
	globalParams := &command.GlobalParams{}
	root := &cobra.Command{Use: "agent"}
	root.PersistentFlags().StringVarP(&globalParams.ConfFilePath, "cfgpath", "c", "", "")
	remote := Commands(globalParams)[0]
	root.AddCommand(remote)

	var discovered bool
	discover := func(remote *cobra.Command, params *command.GlobalParams, _ []string) error {
		discovered = true
		require.Equal(t, "custom.yaml", params.ConfFilePath)
		return AttachCommandProviders(remote, []*pb.CommandProvider{{
			Name:     "fixture-agent",
			Commands: []*pb.Command{{Name: "inspect", IsRunnable: true}},
		}}, func(_ string, _ []string, _ *structpb.Struct, stdout, _ io.Writer) error {
			_, err := io.WriteString(stdout, "executed\n")
			return err
		})
	}

	args := []string{"--cfgpath", "custom.yaml", "remote", "fixture-agent", "inspect"}
	require.NoError(t, prepare(root, args, discover))
	require.True(t, discovered)
	root.SetArgs(args)
	var output bytes.Buffer
	root.SetOut(&output)
	require.NoError(t, root.Execute())
	require.Equal(t, "executed\n", output.String())
}

func TestPrepareSkipsDiscoveryForNonRemoteCommand(t *testing.T) {
	root := &cobra.Command{Use: "agent"}
	root.AddCommand(Commands(&command.GlobalParams{})...)
	root.AddCommand(&cobra.Command{Use: "status"})

	discover := func(*cobra.Command, *command.GlobalParams, []string) error {
		t.Fatal("non-remote command must not discover providers")
		return nil
	}
	require.NoError(t, prepare(root, []string{"status"}, discover))
}

func TestApplyGlobalFlags(t *testing.T) {
	root := command.MakeCommand([]command.SubcommandFactory{Commands})
	remote, _, err := root.Find([]string{"remote"})
	require.NoError(t, err)
	globalParams, ok := remoteGlobalParams.Load(remote)
	require.True(t, ok)

	require.NoError(t, applyGlobalFlags(root, []string{"--cfgpath", "/tmp/config", "remote", "fixture", "--provider-flag", "--extracfgpath=extra.yaml", "--sysprobecfgpath", "/tmp/sysprobe", "--fleetcfgpath=/tmp/fleet", "--no-color"}))
	params := globalParams.(*command.GlobalParams)
	require.Equal(t, "/tmp/config", params.ConfFilePath)
	require.Equal(t, []string{"extra.yaml"}, params.ExtraConfFilePath)
	require.Equal(t, "/tmp/sysprobe", params.SysProbeConfFilePath)
	require.Equal(t, "/tmp/fleet", params.FleetPoliciesDirPath)
	require.True(t, params.NoColor)
}

func TestRemoteCommandDetectionDoesNotSelectOtherCommands(t *testing.T) {
	root := &cobra.Command{Use: "agent"}
	root.AddCommand(Commands(&command.GlobalParams{})...)
	root.AddCommand(&cobra.Command{Use: "status"})

	_, ok := remoteCommand(root, []string{"remote", "data-plane", "dogstatsd", "top"})
	require.True(t, ok)
	_, ok = remoteCommand(root, []string{"status"})
	require.False(t, ok)
}

func TestAttachCommandProvidersBuildsNestedTreeAndPassesTypedFlags(t *testing.T) {
	remote := Commands(&command.GlobalParams{})[0]
	var gotName string
	var gotPath []string
	var gotArguments map[string]any
	top := &pb.Command{
		Name: "dogstatsd.top", ShortName: "top", IsRunnable: true,
		Parameters: []*pb.CommandParameter{
			{Name: "num-metrics", ShortName: "m", Type: pb.ParameterType_TYPE_INT, IsFlag: true, Required: true},
			{Name: "include-inactive", Type: pb.ParameterType_TYPE_BOOL, IsFlag: true},
		},
	}
	providers := []*pb.CommandProvider{{
		Name:        "data-plane",
		Description: "High-performance data plane",
		Commands: []*pb.Command{{
			Name: "dogstatsd", ShortName: "dogstatsd", Children: []*pb.Command{top},
		}},
	}}
	require.NoError(t, AttachCommandProviders(remote, providers, func(name string, path []string, arguments *structpb.Struct, _, _ io.Writer) error {
		gotName, gotPath = name, path
		gotArguments = map[string]any{}
		for key, value := range arguments.GetFields() {
			gotArguments[key] = value.AsInterface()
		}
		return nil
	}))

	provider, _, err := remote.Find([]string{"data-plane"})
	require.NoError(t, err)
	require.Equal(t, "High-performance data plane", provider.Short)
	remote.SetArgs([]string{"data-plane", "dogstatsd", "top", "--num-metrics", "7", "--include-inactive"})
	require.NoError(t, remote.Execute())
	require.Equal(t, "data-plane", gotName)
	require.Equal(t, []string{"dogstatsd", "top"}, gotPath)
	require.Equal(t, float64(7), gotArguments["num-metrics"])
	require.Equal(t, true, gotArguments["include-inactive"])
}

func TestAttachCommandProvidersForwardsInheritedPersistentFlag(t *testing.T) {
	remote := Commands(&command.GlobalParams{})[0]
	var gotArguments map[string]any
	providers := []*pb.CommandProvider{{
		Name: "fixture-agent",
		Commands: []*pb.Command{{
			Name:       "diagnostics",
			Parameters: []*pb.CommandParameter{{Name: "region", Type: pb.ParameterType_TYPE_STRING, IsFlag: true, IsPersistent: true}},
			Children:   []*pb.Command{{Name: "inspect", IsRunnable: true}},
		}},
	}}
	require.NoError(t, AttachCommandProviders(remote, providers, func(_ string, _ []string, arguments *structpb.Struct, _, _ io.Writer) error {
		gotArguments = map[string]any{}
		for key, value := range arguments.GetFields() {
			gotArguments[key] = value.AsInterface()
		}
		return nil
	}))

	remote.SetArgs([]string{"fixture-agent", "diagnostics", "inspect", "--region", "us-east-1"})
	require.NoError(t, remote.Execute())
	require.Equal(t, "us-east-1", gotArguments["region"])
}

func TestAttachCommandProvidersDoesNotForwardNonPersistentParentParameters(t *testing.T) {
	remote := Commands(&command.GlobalParams{})[0]
	var gotArguments map[string]any
	providers := []*pb.CommandProvider{{
		Name: "fixture-agent",
		Commands: []*pb.Command{{
			Name:       "diagnostics",
			Parameters: []*pb.CommandParameter{{Name: "parent-value", Type: pb.ParameterType_TYPE_STRING}},
			Children: []*pb.Command{{
				Name:       "inspect",
				IsRunnable: true,
				Parameters: []*pb.CommandParameter{{Name: "child-value", Type: pb.ParameterType_TYPE_STRING, Required: true}},
			}},
		}},
	}}
	require.NoError(t, AttachCommandProviders(remote, providers, func(_ string, _ []string, arguments *structpb.Struct, _, _ io.Writer) error {
		gotArguments = arguments.AsMap()
		return nil
	}))

	remote.SetArgs([]string{"fixture-agent", "diagnostics", "inspect", "child"})
	require.NoError(t, remote.Execute())
	require.Equal(t, map[string]any{"child-value": "child"}, gotArguments)
}

func TestArgumentsForCommandSupportsFinalPositionalSliceBeforeFlags(t *testing.T) {
	cmd := &cobra.Command{}
	parameters := []*pb.CommandParameter{
		{Name: "items", Type: pb.ParameterType_TYPE_STRING_SLICE},
		{Name: "verbose", Type: pb.ParameterType_TYPE_BOOL, IsFlag: true},
	}
	require.NoError(t, addFlag(cmd, parameters[1]))
	require.NoError(t, cmd.ParseFlags([]string{"--verbose"}))

	arguments, err := argumentsForCommand(cmd, []string{"one", "two"}, parameters)
	require.NoError(t, err)
	require.Equal(t, []any{"one", "two"}, arguments.GetFields()["items"].AsInterface())
	require.Equal(t, true, arguments.GetFields()["verbose"].AsInterface())
}

func TestAttachCommandProvidersRendersResponseAndReturnsExitCode(t *testing.T) {
	remote := Commands(&command.GlobalParams{})[0]
	var stdout, stderr bytes.Buffer
	remote.SetOut(&stdout)
	remote.SetErr(&stderr)
	providers := []*pb.CommandProvider{{
		Name:     "fixture-agent",
		Commands: []*pb.Command{{Name: "inspect", IsRunnable: true}},
	}}
	require.NoError(t, AttachCommandProviders(remote, providers, func(_ string, _ []string, _ *structpb.Struct, stdout, stderr io.Writer) error {
		_, _ = io.WriteString(stdout, "text-out")
		_, _ = io.WriteString(stderr, "text-err")
		_, _ = stdout.Write([]byte{0, 1})
		return exitCodeError(7)
	}))
	remote.SetArgs([]string{"fixture-agent", "inspect"})
	err := remote.Execute()
	require.Error(t, err)
	var exitErr exitCodeError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, 7, exitErr.ExitCode())
	require.Equal(t, "text-out\x00\x01", stdout.String())
	require.Equal(t, "text-err", stderr.String())
}

func TestAttachCommandProvidersRejectsMissingName(t *testing.T) {
	remote := Commands(&command.GlobalParams{})[0]
	require.Error(t, AttachCommandProviders(remote, []*pb.CommandProvider{{}}, func(string, []string, *structpb.Struct, io.Writer, io.Writer) error { return nil }))
}

func TestFlagValueSupportsEveryParameterType(t *testing.T) {
	testCases := []struct {
		parameter *pb.CommandParameter
		argument  string
		expected  any
	}{
		{&pb.CommandParameter{Name: "value", Type: pb.ParameterType_TYPE_STRING}, "--value=text", "text"},
		{&pb.CommandParameter{Name: "value", Type: pb.ParameterType_TYPE_INT}, "--value=7", float64(7)},
		{&pb.CommandParameter{Name: "value", Type: pb.ParameterType_TYPE_UINT}, "--value=7", float64(7)},
		{&pb.CommandParameter{Name: "value", Type: pb.ParameterType_TYPE_FLOAT}, "--value=1.5", 1.5},
		{&pb.CommandParameter{Name: "value", Type: pb.ParameterType_TYPE_BOOL}, "--value", true},
		{&pb.CommandParameter{Name: "value", Type: pb.ParameterType_TYPE_STRING_SLICE}, "--value=a,b", []any{"a", "b"}},
		{&pb.CommandParameter{Name: "value", Type: pb.ParameterType_TYPE_INT_SLICE}, "--value=1,2", []any{float64(1), float64(2)}},
		{&pb.CommandParameter{Name: "value", Type: pb.ParameterType_TYPE_UINT_SLICE}, "--value=1,2", []any{float64(1), float64(2)}},
		{&pb.CommandParameter{Name: "value", Type: pb.ParameterType_TYPE_FLOAT_SLICE}, "--value=1.5,2.5", []any{1.5, 2.5}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.parameter.GetType().String(), func(t *testing.T) {
			cmd := &cobra.Command{}
			require.NoError(t, addFlag(cmd, testCase.parameter))
			require.NoError(t, cmd.ParseFlags([]string{testCase.argument}))
			value, err := flagValue(cmd, testCase.parameter)
			require.NoError(t, err)
			require.Equal(t, testCase.expected, value.AsInterface())
		})
	}
}

func TestUnsupportedParameterTypeReturnsError(t *testing.T) {
	parameter := &pb.CommandParameter{Name: "value", Type: pb.ParameterType_TYPE_UNSPECIFIED}
	require.Error(t, addFlag(&cobra.Command{}, parameter))
	_, err := stringValue(parameter.GetType(), "value")
	require.Error(t, err)
}
