// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotecommand

import (
	"bytes"
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

func TestPrepareFxDependencies(t *testing.T) {
	root := &cobra.Command{Use: "agent"}
	root.AddCommand(Commands(&command.GlobalParams{})...)

	fxutil.TestOneShot(t, func() {
		require.NoError(t, Prepare(root, []string{"remote"}))
	})
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
	var gotName, gotPath string
	var gotArguments map[string]any
	top := &pb.Command{
		Name: "dogstatsd.top", ShortName: "top", IsRunnable: true,
		Parameters: []*pb.CommandParameter{
			{Name: "num-metrics", ShortName: "m", Type: pb.ParameterType_TYPE_INT, IsFlag: true, Required: true},
			{Name: "include-inactive", Type: pb.ParameterType_TYPE_BOOL, IsFlag: true},
		},
	}
	providers := []*pb.CommandProvider{{
		CommandName:      "data-plane",
		AgentDescription: "High-performance data plane",
		Commands: []*pb.Command{{
			Name: "dogstatsd", ShortName: "dogstatsd", Children: []*pb.Command{top},
		}},
	}}
	require.NoError(t, AttachCommandProviders(remote, providers, func(name, path string, arguments *structpb.Struct) (*pb.ExecuteCommandResponse, error) {
		gotName, gotPath = name, path
		gotArguments = map[string]any{}
		for key, value := range arguments.GetFields() {
			gotArguments[key] = value.AsInterface()
		}
		return &pb.ExecuteCommandResponse{}, nil
	}))

	provider, _, err := remote.Find([]string{"data-plane"})
	require.NoError(t, err)
	require.Equal(t, "High-performance data plane", provider.Short)
	remote.SetArgs([]string{"data-plane", "dogstatsd", "top", "--num-metrics", "7", "--include-inactive"})
	require.NoError(t, remote.Execute())
	require.Equal(t, "data-plane", gotName)
	require.Equal(t, "dogstatsd.top", gotPath)
	require.Equal(t, float64(7), gotArguments["num-metrics"])
	require.Equal(t, true, gotArguments["include-inactive"])
}

func TestAttachCommandProvidersRendersResponseAndReturnsExitCode(t *testing.T) {
	remote := Commands(&command.GlobalParams{})[0]
	var stdout, stderr bytes.Buffer
	remote.SetOut(&stdout)
	remote.SetErr(&stderr)
	providers := []*pb.CommandProvider{{
		CommandName: "fixture-agent",
		Commands:    []*pb.Command{{Name: "inspect", IsRunnable: true}},
	}}
	require.NoError(t, AttachCommandProviders(remote, providers, func(string, string, *structpb.Struct) (*pb.ExecuteCommandResponse, error) {
		return &pb.ExecuteCommandResponse{Stdout: "text-out", Stderr: "text-err", BinaryOutput: []byte{0, 1}, ExitCode: 7}, nil
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
	require.Error(t, AttachCommandProviders(remote, []*pb.CommandProvider{{}}, func(string, string, *structpb.Struct) (*pb.ExecuteCommandResponse, error) { return nil, nil }))
}
