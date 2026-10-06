// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package taskverifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

func localRemediationTask() *types.Task {
	task := &types.Task{}
	task.Data.ID = "local-task"
	task.Data.Attributes = &types.Attributes{
		BundleID: "com.datadoghq.remoteaction.rshell",
		Name:     "runRemediationCommand",
		Inputs:   map[string]interface{}{"command": "touch /tmp/remediation/result"},
	}
	return task
}

func TestLocalTrustVerifierCopiesLocalPolicyAndIdentity(t *testing.T) {
	policy := &privateactionspb.RemoteAction{
		AllowedCommands: []string{"rshell:touch"},
		AllowedPaths:    []string{"/tmp/remediation"},
		SystemServices: map[string]*structpb.ListValue{
			"example.service": {Values: []*structpb.Value{structpb.NewStringValue("restart")}},
		},
	}
	task := localRemediationTask()
	task.Data.Attributes.SystemInputs = &privateactionspb.SystemInputs{Input: &privateactionspb.SystemInputs_RemoteAction{RemoteAction: policy}}
	task.Data.Attributes.OrgId = 123
	task.Data.Attributes.ConnectionInfo = &privateactionspb.ConnectionInfo{RunnerId: "untrusted"}
	verifier := NewLocalTrustVerifier(&config.Config{OrgId: 42, RunnerId: "local-runner"})

	got, err := verifier.UnwrapTask(task)

	require.NoError(t, err)
	assert.Equal(t, int64(42), got.Data.Attributes.OrgId)
	assert.Equal(t, "local-runner", got.Data.Attributes.ConnectionInfo.RunnerId)
	assert.Nil(t, got.Data.Attributes.SignedEnvelope)
	assert.Nil(t, got.Data.Attributes.VerificationKey)
	assert.True(t, proto.Equal(policy, got.Data.Attributes.SystemInputs.GetRemoteAction()))
	policy.AllowedCommands[0] = "rshell:rm"
	policy.SystemServices["example.service"].Values[0] = structpb.NewStringValue("stop")
	task.Data.Attributes.Inputs["command"] = "rm /tmp/remediation/result"
	assert.Equal(t, []string{"rshell:touch"}, got.Data.Attributes.SystemInputs.GetRemoteAction().AllowedCommands)
	assert.Equal(t, "restart", got.Data.Attributes.SystemInputs.GetRemoteAction().SystemServices["example.service"].Values[0].GetStringValue())
	assert.Equal(t, "touch /tmp/remediation/result", got.Data.Attributes.Inputs["command"])
	assert.Equal(t, int64(123), task.Data.Attributes.OrgId)
}

func TestLocalTrustVerifierDefaultsToEmptyPolicy(t *testing.T) {
	got, err := NewLocalTrustVerifier(&config.Config{}).UnwrapTask(localRemediationTask())
	require.NoError(t, err)
	policy := got.Data.Attributes.SystemInputs.GetRemoteAction()
	require.NotNil(t, policy)
	assert.Empty(t, policy.AllowedCommands)
	assert.Empty(t, policy.AllowedPaths)
	assert.Empty(t, policy.SystemServices)
}

func TestLocalTrustVerifierRejectsForeignAuthority(t *testing.T) {
	cases := map[string]func(*types.Task){
		"missing attributes": func(task *types.Task) { task.Data.Attributes = nil },
		"other bundle":       func(task *types.Task) { task.Data.Attributes.BundleID = "com.datadoghq.http" },
		"other action":       func(task *types.Task) { task.Data.Attributes.Name = "runCommand" },
		"signed envelope": func(task *types.Task) {
			task.Data.Attributes.SignedEnvelope = &privateactionspb.RemoteConfigSignatureEnvelope{}
		},
		"verification key": func(task *types.Task) { task.Data.Attributes.VerificationKey = &types.TaskVerificationKey{} },
		"connection": func(task *types.Task) {
			task.Data.Attributes.ConnectionInfo = &privateactionspb.ConnectionInfo{ConnectionId: "connection"}
		},
		"credential token": func(task *types.Task) {
			task.Data.Attributes.ConnectionInfo = &privateactionspb.ConnectionInfo{Tokens: []*privateactionspb.ConnectionToken{{}}}
		},
		"credentials type": func(task *types.Task) {
			task.Data.Attributes.ConnectionInfo = &privateactionspb.ConnectionInfo{CredentialsType: privateactionspb.CredentialsType(1)}
		},
		"security header": func(task *types.Task) { task.Data.Attributes.SecDatadogHeaderValue = "header" },
		"escalation": func(task *types.Task) {
			task.Data.Attributes.Inputs["effectivePermissions"] = "EscalationAllowed"
		},
		"root": func(task *types.Task) { task.Data.Attributes.Inputs["effectivePermissions"] = "Root" },
		"case variation": func(task *types.Task) {
			task.Data.Attributes.Inputs["EffectivePermissions"] = "EscalationAllowed"
		},
		"elevatable commands": func(task *types.Task) {
			task.Data.Attributes.Inputs["elevatableCommands"] = []string{"rshell:touch"}
		},
		"JSON elevatable commands": func(task *types.Task) {
			task.Data.Attributes.Inputs["elevatableCommands"] = []interface{}{"rshell:touch"}
		},
		"malformed permissions": func(task *types.Task) { task.Data.Attributes.Inputs["effectivePermissions"] = true },
		"malformed elevation":   func(task *types.Task) { task.Data.Attributes.Inputs["elevatableCommands"] = "rshell:touch" },
		"unknown input":         func(task *types.Task) { task.Data.Attributes.Inputs["allowedCommands"] = []string{"*"} },
		"empty command":         func(task *types.Task) { task.Data.Attributes.Inputs["command"] = " " },
	}
	verifier := NewLocalTrustVerifier(&config.Config{})
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			task := localRemediationTask()
			mutate(task)
			got, err := verifier.UnwrapTask(task)
			require.Error(t, err)
			assert.Nil(t, got)
		})
	}
	_, err := verifier.UnwrapTask(nil)
	require.Error(t, err)
}

func TestLocalTrustVerifierRejectsBroadPolicy(t *testing.T) {
	cases := map[string]*privateactionspb.RemoteAction{
		"wildcard commands":    {AllowedCommands: []string{"rshell:*"}},
		"unnamespaced command": {AllowedCommands: []string{"touch"}},
		"empty command":        {AllowedCommands: []string{"rshell:"}},
		"command arguments":    {AllowedCommands: []string{"rshell:touch /tmp"}},
		"wildcard paths":       {AllowedPaths: []string{"/tmp/*"}},
		"empty path":           {AllowedPaths: []string{""}},
		"glob path":            {AllowedPaths: []string{"/tmp/[ab]"}},
		"wildcard service":     {SystemServices: map[string]*structpb.ListValue{"*": {}}},
		"wildcard action": {SystemServices: map[string]*structpb.ListValue{
			"example.service": {Values: []*structpb.Value{structpb.NewStringValue("*")}},
		}},
		"non-string action": {SystemServices: map[string]*structpb.ListValue{
			"example.service": {Values: []*structpb.Value{structpb.NewNumberValue(1)}},
		}},
	}
	for name, policy := range cases {
		t.Run(name, func(t *testing.T) {
			task := localRemediationTask()
			task.Data.Attributes.SystemInputs = &privateactionspb.SystemInputs{Input: &privateactionspb.SystemInputs_RemoteAction{RemoteAction: policy}}
			_, err := NewLocalTrustVerifier(&config.Config{}).UnwrapTask(task)
			require.Error(t, err)
		})
	}
}

func TestSigningVerifierStillRejectsUnsignedTasks(t *testing.T) {
	_, err := NewTaskVerifier(nil, &config.Config{}).UnwrapTask(localRemediationTask())
	require.ErrorContains(t, err, "task is missing signed envelope")
}
