// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package taskverifier

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

func TestMapPbTaskToStructMapsRemoteActionPolicyFields(t *testing.T) {
	task := &privateactionspb.PrivateActionTask{
		ActionName: "runCommand",
		BundleId:   "com.datadoghq.remoteaction.rshell",
		OrgId:      42,
		TaskId:     "task-id",
		Inputs:     &structpb.Struct{},
		SystemInputs: &privateactionspb.SystemInputs{
			Input: &privateactionspb.SystemInputs_RemoteAction{
				RemoteAction: &privateactionspb.RemoteAction{
					AllowedCommands: []string{"rshell:cat"},
					AllowedPaths:    []string{"/host/var/log"},
					SystemServices: map[string]*structpb.ListValue{
						"mysql.service": stringListValue("read", "restart"),
					},
				},
			},
		},
	}

	got := mapPbTaskToStruct(task)

	assert.Equal(t, "task-id", got.Data.ID)
	assert.Equal(t, "runCommand", got.Data.Attributes.Name)
	require.NotNil(t, got.Data.Attributes.SystemInputs)
	remoteAction := got.Data.Attributes.SystemInputs.GetRemoteAction()
	require.NotNil(t, remoteAction)
	assert.Equal(t, []string{"rshell:cat"}, remoteAction.AllowedCommands)
	assert.Equal(t, []string{"/host/var/log"}, remoteAction.AllowedPaths)
	require.Contains(t, remoteAction.SystemServices, "mysql.service")
	assert.Equal(t, []interface{}{"read", "restart"}, remoteAction.SystemServices["mysql.service"].AsSlice())
}

func TestMapPbTaskToStructEmptyRemoteActionPolicyFields(t *testing.T) {
	got := mapPbTaskToStruct(&privateactionspb.PrivateActionTask{Inputs: &structpb.Struct{}})

	assert.Nil(t, got.Data.Attributes.SystemInputs)
}

func TestNoOpTaskVerifierUnwrapsSignedEnvelopeData(t *testing.T) {
	inputs, err := structpb.NewStruct(map[string]interface{}{"command": "cat /tmp/file"})
	require.NoError(t, err)
	pbTask := &privateactionspb.PrivateActionTask{
		ActionName: "runCommand",
		BundleId:   "com.datadoghq.remoteaction.rshell",
		TaskId:     "task-id",
		Inputs:     inputs,
		SystemInputs: &privateactionspb.SystemInputs{
			Input: &privateactionspb.SystemInputs_RemoteAction{
				RemoteAction: &privateactionspb.RemoteAction{
					AllowedCommands: []string{"rshell:cat"},
					AllowedPaths:    []string{"/tmp:ro"},
					SystemServices: map[string]*structpb.ListValue{
						"nginx.service": stringListValue("read"),
					},
				},
			},
		},
	}
	signedTaskData, err := proto.Marshal(pbTask)
	require.NoError(t, err)

	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		SignedEnvelope: &privateactionspb.RemoteConfigSignatureEnvelope{
			Data: signedTaskData,
		},
	}

	got, err := (&noOpTaskVerifier{}).UnwrapTask(task)

	require.NoError(t, err)
	assert.Equal(t, "task-id", got.Data.ID)
	remoteAction := got.Data.Attributes.SystemInputs.GetRemoteAction()
	require.NotNil(t, remoteAction)
	assert.Equal(t, []string{"rshell:cat"}, remoteAction.AllowedCommands)
	assert.Equal(t, []string{"/tmp:ro"}, remoteAction.AllowedPaths)
	require.Contains(t, remoteAction.SystemServices, "nginx.service")
	assert.Equal(t, []interface{}{"read"}, remoteAction.SystemServices["nginx.service"].AsSlice())
}

func stringListValue(values ...string) *structpb.ListValue {
	list := &structpb.ListValue{Values: make([]*structpb.Value, 0, len(values))}
	for _, value := range values {
		list.Values = append(list.Values, structpb.NewStringValue(value))
	}
	return list
}
