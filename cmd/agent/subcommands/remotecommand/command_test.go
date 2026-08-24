// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remotecommand

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/cmd/agent/command"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRunListCommandsFxDependencies(t *testing.T) {
	fxutil.TestOneShot(t, func() {
		runListCommands(&cliParams{GlobalParams: &command.GlobalParams{}})
	})
}

func TestRunExecuteCommandFxDependencies(t *testing.T) {
	fxutil.TestOneShot(t, func() {
		runExecuteCommand(&cliParams{GlobalParams: &command.GlobalParams{}, agentID: "agent-id", argumentsJSON: "{}"}, "agent.status")
	})
}

func TestParseArguments(t *testing.T) {
	arguments, err := parseArguments(`{"name":"value","count":2,"enabled":true,"values":[1,2]}`)
	require.NoError(t, err)
	require.Equal(t, "value", arguments.GetFields()["name"].GetStringValue())
	require.Equal(t, 2.0, arguments.GetFields()["count"].GetNumberValue())
	require.True(t, arguments.GetFields()["enabled"].GetBoolValue())
	require.Len(t, arguments.GetFields()["values"].GetListValue().GetValues(), 2)

	for _, raw := range []string{"not-json", `[]`, `null`} {
		_, err := parseArguments(raw)
		require.Error(t, err, raw)
	}
}

func TestFindCommandRecursivelyMatchesAgentAndPath(t *testing.T) {
	commands := []*pb.Command{{
		Name: "parent", AgentId: "agent-a",
		Children: []*pb.Command{{Name: "parent.child", AgentId: "agent-a"}},
	}, {Name: "parent.child", AgentId: "agent-b"}}

	require.Equal(t, "agent-a", findCommand(commands, "agent-a", "parent.child").GetAgentId())
	require.Equal(t, "agent-b", findCommand(commands, "agent-b", "parent.child").GetAgentId())
	require.Nil(t, findCommand(commands, "agent-c", "parent.child"))
	require.Nil(t, findCommand(commands, "agent-a", "missing"))
}

func TestFindCommandIncludesInheritedPersistentParameters(t *testing.T) {
	leaf := &pb.Command{Name: "parent.child", AgentId: "agent-a", IsRunnable: true}
	commands := []*pb.Command{{
		Name: "parent", AgentId: "agent-a",
		Parameters: []*pb.CommandParameter{
			{Name: "persistent", Type: pb.ParameterType_TYPE_STRING, IsPersistent: true},
			{Name: "parent-only", Type: pb.ParameterType_TYPE_STRING},
		},
		Children: []*pb.Command{leaf},
	}}
	found := findCommand(commands, "agent-a", "parent.child")
	require.NotNil(t, found)
	require.NoError(t, validateArguments(found, mustArguments(t, `{"persistent":"value"}`)))
	require.Error(t, validateArguments(found, mustArguments(t, `{"parent-only":"value"}`)))
}

func TestFlattenCommandsIncludesNestedCommands(t *testing.T) {
	commands := []*pb.Command{{Name: "parent", Children: []*pb.Command{{Name: "parent.child"}}}}
	flattened := flattenCommands(commands)
	require.Len(t, flattened, 2)
	require.Equal(t, "parent", flattened[0].GetName())
	require.Equal(t, "parent.child", flattened[1].GetName())
}

func mustArguments(t *testing.T, raw string) *structpb.Struct {
	t.Helper()
	arguments, err := parseArguments(raw)
	require.NoError(t, err)
	return arguments
}

func TestValidateArguments(t *testing.T) {
	command := &pb.Command{Parameters: []*pb.CommandParameter{
		{Name: "name", Type: pb.ParameterType_TYPE_STRING, Required: true},
		{Name: "count", Type: pb.ParameterType_TYPE_INT},
		{Name: "enabled", Type: pb.ParameterType_TYPE_BOOL},
		{Name: "labels", Type: pb.ParameterType_TYPE_STRING_SLICE},
		{Name: "ratios", Type: pb.ParameterType_TYPE_FLOAT_SLICE},
	}}
	valid, err := parseArguments(`{"name":"test","count":2,"enabled":false,"labels":["a","b"],"ratios":[1.5,2]}`)
	require.NoError(t, err)
	require.NoError(t, validateArguments(command, valid))

	for name, raw := range map[string]string{
		"missing required": `{}`,
		"unknown":          `{"name":"test","unknown":1}`,
		"wrong scalar":     `{"name":"test","count":"two"}`,
		"wrong slice":      `{"name":"test","labels":[1]}`,
		"non-integral int": `{"name":"test","count":1.5}`,
	} {
		arguments, err := parseArguments(raw)
		require.NoError(t, err, name)
		require.Error(t, validateArguments(command, arguments), name)
	}
}

func TestValidateArgumentsSupportsAllDeclaredTypes(t *testing.T) {
	for typ, value := range map[pb.ParameterType]*structpb.Value{
		pb.ParameterType_TYPE_STRING:       structpb.NewStringValue("value"),
		pb.ParameterType_TYPE_INT:          structpb.NewNumberValue(-1),
		pb.ParameterType_TYPE_UINT:         structpb.NewNumberValue(1),
		pb.ParameterType_TYPE_FLOAT:        structpb.NewNumberValue(1.5),
		pb.ParameterType_TYPE_BOOL:         structpb.NewBoolValue(true),
		pb.ParameterType_TYPE_STRING_SLICE: structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("value")}}),
		pb.ParameterType_TYPE_INT_SLICE:    structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewNumberValue(-1)}}),
		pb.ParameterType_TYPE_UINT_SLICE:   structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewNumberValue(1)}}),
		pb.ParameterType_TYPE_FLOAT_SLICE:  structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewNumberValue(1.5)}}),
	} {
		require.True(t, matchesType(typ, value), typ.String())
	}
}

func TestCommandArgumentContracts(t *testing.T) {
	commands := Commands(&command.GlobalParams{})
	remote := commands[0]
	list, _, err := remote.Find([]string{"list"})
	require.NoError(t, err)
	require.NoError(t, list.Args(list, nil))
	require.Error(t, list.Args(list, []string{"extra"}))

	execute, _, err := remote.Find([]string{"execute"})
	require.NoError(t, err)
	require.Error(t, execute.Args(execute, nil))
	require.NoError(t, execute.Args(execute, []string{"agent.status"}))
	require.Error(t, execute.Args(execute, []string{"one", "two"}))
	require.NotNil(t, execute.Flags().Lookup("agent-id"))
	require.NotNil(t, execute.Flags().Lookup("arguments"))
}
