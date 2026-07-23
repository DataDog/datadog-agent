// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package taskverifier

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

type staticKeysManager struct {
	keys map[string]types.DecodedKey
}

func (m *staticKeysManager) Start(context.Context) {}
func (m *staticKeysManager) GetKey(id string) types.DecodedKey {
	return m.keys[id]
}
func (m *staticKeysManager) WaitForReady() {}

func TestSignedEnvelopeVerifierAttachesAuthenticatedPublicKey(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	pbTask := &privateactionspb.PrivateActionTask{
		OrgId:          42,
		TaskId:         "task-id",
		Inputs:         &structpb.Struct{},
		ConnectionInfo: &privateactionspb.ConnectionInfo{RunnerId: "runner-id"},
		ExpirationTime: timestamppb.New(time.Now().Add(time.Minute)),
	}
	data, err := proto.Marshal(pbTask)
	require.NoError(t, err)
	digest := sha256.Sum256(data)
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{SignedEnvelope: &privateactionspb.RemoteConfigSignatureEnvelope{
		Data: data, HashType: privateactionspb.HashType_SHA256,
		Signatures: []*privateactionspb.Signature{{
			KeyId: "key-id", KeyType: privateactionspb.KeyType_ED25519,
			Signature: ed25519.Sign(private, digest[:]),
		}},
	}}
	verifier := &signedEnvelopeTaskVerifier{
		keysManager: &staticKeysManager{keys: map[string]types.DecodedKey{
			"key-id": &types.ED25519Key{KeyType: types.KeyTypeED25519, Key: public},
		}},
		config: &config.Config{OrgId: 42, RunnerId: "runner-id"},
	}

	got, err := verifier.UnwrapTask(task)

	require.NoError(t, err)
	require.NotNil(t, got.Data.Attributes.VerificationKey)
	assert.Equal(t, "key-id", got.Data.Attributes.VerificationKey.ID)
	assert.Equal(t, types.KeyTypeED25519, got.Data.Attributes.VerificationKey.KeyType)
	assert.Contains(t, got.Data.Attributes.VerificationKey.PEM, "BEGIN PUBLIC KEY")
}

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
}
