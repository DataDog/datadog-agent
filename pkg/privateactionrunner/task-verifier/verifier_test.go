// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package taskverifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		RemoteAction: &privateactionspb.RemoteAction{
			TargetCommands: []string{"rshell:cat"},
			TargetPaths:    []string{"/host/var/log"},
		},
	}

	got := mapPbTaskToStruct(task)

	assert.Equal(t, "task-id", got.Data.ID)
	assert.Equal(t, "runCommand", got.Data.Attributes.Name)
	require.NotNil(t, got.Data.Attributes.RemoteAction)
	assert.Equal(t, []string{"rshell:cat"}, got.Data.Attributes.RemoteAction.TargetCommands)
	assert.Equal(t, []string{"/host/var/log"}, got.Data.Attributes.RemoteAction.TargetPaths)
}

func TestMapPbTaskToStructEmptyRemoteActionPolicyFields(t *testing.T) {
	got := mapPbTaskToStruct(&privateactionspb.PrivateActionTask{Inputs: &structpb.Struct{}})

	assert.Nil(t, got.Data.Attributes.RemoteAction)
}
