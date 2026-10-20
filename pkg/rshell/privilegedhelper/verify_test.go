// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privilegedhelper

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func signedRequest(t *testing.T, private ed25519.PrivateKey, mutate func(*PrivateActionTask)) ExecuteRequest {
	t.Helper()
	inputs, err := structpb.NewStruct(map[string]any{
		"command":              "sudo truncate -s 0 /var/log/app.log",
		"effectivePermissions": EscalationAllowed,
		"elevatableCommands":   []any{"rshell:truncate"},
	})
	require.NoError(t, err)
	task := &PrivateActionTask{
		ActionName: remediationAction, BundleId: rshellBundle, OrgId: 42, TaskId: "task-1",
		Inputs: inputs, ConnectionInfo: &ConnectionInfo{RunnerId: "runner-1"},
		ExpirationTime: timestamppb.New(time.Now().Add(time.Minute)),
		SystemInputs: &SystemInputs{Input: &SystemInputs_RemoteAction{RemoteAction: &RemoteAction{
			AllowedCommands: []string{"rshell:truncate", "rshell:echo"}, AllowedPaths: []string{"/var/log"},
		}}},
	}
	if mutate != nil {
		mutate(task)
	}
	data, err := proto.Marshal(task)
	require.NoError(t, err)
	digest := sha256.Sum256(data)
	return ExecuteRequest{Version: ProtocolVersion, Envelope: SignedEnvelope{
		Data: data, HashType: "SHA256", Signatures: []Signature{{KeyType: KeyTypeED25519, KeyID: "key-1", Signature: ed25519.Sign(private, digest[:])}},
	}}
}

func systemServiceActions(t *testing.T, actions ...any) *structpb.ListValue {
	t.Helper()
	list, err := structpb.NewList(actions)
	require.NoError(t, err)
	return list
}

func testCredential(t *testing.T) (*Credential, ed25519.PrivateKey) {
	t.Helper()
	public, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	return &Credential{
		Version: ProtocolVersion, OrgID: 42, RunnerID: "runner-1",
		AllowedCommands: []string{"rshell:truncate"}, AllowedPaths: []string{"/var/log"}, ElevatableCommands: []string{"rshell:truncate"},
		decodedKeys: map[string]verificationKey{"key-1": ed25519Key{key: public}},
	}, private
}

func socketCredentialKey(t *testing.T, private ed25519.PrivateKey) CredentialKey {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(private.Public())
	require.NoError(t, err)
	return CredentialKey{
		ID: "key-1", Type: KeyTypeED25519,
		PEM: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})),
	}
}

func TestVerifySignedRequest(t *testing.T) {
	credential, private := testCredential(t)
	verified, err := credential.Verify(signedRequest(t, private, nil), time.Now())
	require.NoError(t, err)
	require.Equal(t, "task-1", verified.TaskID)
	require.Equal(t, ExecutionModeRemediation, verified.Mode)
	require.Equal(t, []string{"rshell:truncate"}, verified.AllowedCommands)
	require.Equal(t, []string{"rshell:truncate"}, verified.ElevatableCommands)
}

func TestVerifySignedReadOnlyRequest(t *testing.T) {
	credential, private := testCredential(t)
	verified, err := credential.Verify(signedRequest(t, private, func(task *PrivateActionTask) {
		task.ActionName = readOnlyAction
	}), time.Now())
	require.NoError(t, err)
	require.Equal(t, ExecutionModeReadOnly, verified.Mode)
}

func TestRequestCredentialUsesSignedBackendPolicy(t *testing.T) {
	_, private := testCredential(t)
	credential, err := NewRequestCredential([]CredentialKey{socketCredentialKey(t, private)})
	require.NoError(t, err)

	verified, err := credential.Verify(signedRequest(t, private, func(task *PrivateActionTask) {
		task.GetSystemInputs().GetRemoteAction().SystemServices = map[string]*structpb.ListValue{
			"mysql.service": systemServiceActions(t, "read", "restart"),
		}
	}), time.Now())
	require.NoError(t, err)
	require.Equal(t, []string{"rshell:truncate", "rshell:echo"}, verified.AllowedCommands)
	require.Equal(t, []string{"/var/log"}, verified.AllowedPaths)
	require.Equal(t, map[string][]string{"mysql.service": {"read", "restart"}}, verified.AllowedSystemServices)
	require.Equal(t, []string{"rshell:truncate"}, verified.ElevatableCommands)
}

func TestRequestCredentialNormalizesSignedBackendAllowedPaths(t *testing.T) {
	_, private := testCredential(t)
	credential, err := NewRequestCredential([]CredentialKey{socketCredentialKey(t, private)})
	require.NoError(t, err)

	verified, err := credential.Verify(signedRequest(t, private, func(task *PrivateActionTask) {
		task.GetSystemInputs().GetRemoteAction().AllowedPaths = []string{"/:ro", "/:rw"}
	}), time.Now())
	require.NoError(t, err)
	require.True(t, credential.trustBackendPolicy)
	require.Equal(t, []string{"/:rw"}, verified.AllowedPaths)
	require.Equal(t, []string{"/:ro", "/:rw"}, verified.authorization.Signed.AllowedPaths)
	require.Equal(t, []string{"/:rw"}, verified.authorization.Effective.AllowedPaths)
}

func TestServerWithoutCredentialUsesBareRequestKey(t *testing.T) {
	_, private := testCredential(t)
	executor := &testExecutor{}
	server := &Server{Executor: executor}
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	go server.handle(context.Background(), serverConn)

	request := signedRequest(t, private, nil)
	request.VerificationKeys = []CredentialKey{
		{ID: "ignored-director-proof", Type: KeyTypeTUFDirector, PEM: "not parsed without a local trust root"},
		socketCredentialKey(t, private),
	}
	require.NoError(t, writeMessage(clientConn, request))
	var response ExecuteResponse
	require.NoError(t, readMessage(clientConn, &response))

	require.Empty(t, response.Error)
	require.Equal(t, 23, response.ExitCode)
	require.Equal(t, []string{"rshell:truncate", "rshell:echo"}, executor.command.AllowedCommands)
}

func TestPolicyWithoutTrustMaterialNarrowsSignedBackendPolicy(t *testing.T) {
	_, private := testCredential(t)
	executor := &testExecutor{}
	server := &Server{
		Credential: &Credential{
			Version:               ProtocolVersion,
			AllowedCommands:       []string{"rshell:truncate"},
			AllowedPaths:          []string{"/var/log:ro"},
			AllowedSystemServices: map[string][]string{"mysql.service": {"read"}},
			ElevatableCommands:    []string{"rshell:truncate"},
		},
		Executor: executor,
	}
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	go server.handle(context.Background(), serverConn)

	request := signedRequest(t, private, func(task *PrivateActionTask) {
		task.GetSystemInputs().GetRemoteAction().SystemServices = map[string]*structpb.ListValue{
			"mysql.service":   systemServiceActions(t, "read", "restart"),
			"ignored.service": systemServiceActions(t, "read"),
		}
	})
	request.VerificationKeys = []CredentialKey{socketCredentialKey(t, private)}
	require.NoError(t, writeMessage(clientConn, request))
	var response ExecuteResponse
	require.NoError(t, readMessage(clientConn, &response))

	require.Empty(t, response.Error)
	require.Equal(t, []string{"rshell:truncate"}, executor.command.AllowedCommands)
	require.Equal(t, []string{"/var/log"}, executor.command.AllowedPaths)
	require.Equal(t, map[string][]string{"mysql.service": {"read"}}, executor.command.AllowedSystemServices)
	require.Equal(t, []string{"rshell:truncate"}, executor.command.ElevatableCommands)
}

func TestIntersectSystemServicesHonorsActionWildcards(t *testing.T) {
	requested := map[string][]string{
		"all-signed.service": {"*"},
		"all-local.service":  {"read", "restart"},
	}
	configured := map[string][]string{
		"all-signed.service": {"read", "stop"},
		"all-local.service":  {"*"},
	}
	require.Equal(t, map[string][]string{
		"all-signed.service": {"read", "stop"},
		"all-local.service":  {"read", "restart"},
	}, intersectSystemServices(requested, configured))
}

func TestIntersectPathsCollapsesDuplicateModes(t *testing.T) {
	require.Equal(t,
		[]string{"/var/log:rw"},
		intersectPaths([]string{"/:ro", "/:rw"}, []string{"/var/log:rw"}),
	)
	require.Equal(t,
		[]string{"/var/log:rw"},
		intersectPaths([]string{"/:rw", "/:ro"}, []string{"/var/log:rw"}),
	)
}

func TestNormalizeEffectivePaths(t *testing.T) {
	require.Equal(t,
		[]string{"/:rw"},
		normalizeEffectivePaths([]string{"/:ro", "/:rw"}),
	)
	require.Equal(t,
		[]string{"/:rw"},
		normalizeEffectivePaths([]string{"/:rw", "/:ro"}),
	)
	require.Equal(t,
		[]string{"/tmp:rw", "/tmp/readonly:ro"},
		normalizeEffectivePaths([]string{"/tmp:rw", "/tmp/readonly:ro"}),
	)
}

type testExecutor struct {
	command *VerifiedCommand
}

func (e *testExecutor) Execute(_ context.Context, command *VerifiedCommand) (*ExecuteResponse, error) {
	e.command = command
	return &ExecuteResponse{ExitCode: 23}, nil
}

func TestServerUsesDirectorAuthenticatedKeyForOneRequest(t *testing.T) {
	credential, private := testCredential(t)
	root, material := directorMaterial(t, 42, "key-1", private.Public().(ed25519.PublicKey))
	credential.DirectorRoot = root
	credential.decodedKeys = map[string]verificationKey{}
	executor := &testExecutor{}
	server := &Server{Credential: credential, Executor: executor}
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	go server.handle(context.Background(), serverConn)

	request := signedRequest(t, private, nil)
	request.VerificationKeys = []CredentialKey{material, socketCredentialKey(t, private)}
	require.NoError(t, writeMessage(clientConn, request))
	var response ExecuteResponse
	require.NoError(t, readMessage(clientConn, &response))

	require.Empty(t, response.Error)
	require.Equal(t, 23, response.ExitCode)
	require.Equal(t, "task-1", executor.command.TaskID)
	_, err := credential.Verify(request, time.Now())
	require.EqualError(t, err, "no trusted signature found")
}

func TestServerLogsAuthorizationPolicyIntersection(t *testing.T) {
	credential, private := testCredential(t)
	credential.ElevatableCommands = nil
	executor := &testExecutor{}
	var diagnostics bytes.Buffer
	server := &Server{Credential: credential, Executor: executor, LogWriter: &diagnostics}
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	go server.handle(context.Background(), serverConn)

	require.NoError(t, writeMessage(clientConn, signedRequest(t, private, nil)))
	var response ExecuteResponse
	require.NoError(t, readMessage(clientConn, &response))

	require.Empty(t, response.Error)
	require.Empty(t, executor.command.ElevatableCommands)
	logged := diagnostics.String()
	require.Contains(t, logged, `"event":"authorization_context"`)
	require.Contains(t, logged, `"taskId":"task-1"`)
	require.Contains(t, logged, `"orgId":42`)
	require.Contains(t, logged, `"runnerId":"runner-1"`)
	require.Contains(t, logged, `"effectivePermissions":"EscalationAllowed"`)
	require.Contains(t, logged, `"signed":{"allowedCommands":["rshell:truncate","rshell:echo"],"allowedPaths":["/var/log"],"elevatableCommands":["rshell:truncate"]}`)
	require.Contains(t, logged, `"local":{"allowedCommands":["rshell:truncate"],"allowedPaths":["/var/log"],"elevatableCommands":null}`)
	require.Contains(t, logged, `"effective":{"allowedCommands":["rshell:truncate"],"allowedPaths":["/var/log"],"elevatableCommands":[]}`)
	require.Contains(t, logged, `"event":"execution_completed"`)
	require.NotContains(t, logged, "sudo truncate")
	require.NotContains(t, logged, "BEGIN PUBLIC KEY")
}

func TestDecodeX509RSAAcceptsAgentPublicKeyPEM(t *testing.T) {
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&private.PublicKey)
	require.NoError(t, err)

	key, err := decodeKey(CredentialKey{
		ID: "rsa-key", Type: KeyTypeX509RSA,
		PEM: string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})),
	})

	require.NoError(t, err)
	require.Equal(t, KeyTypeX509RSA, key.keyType())
}

func TestVerifySignedInputTypesFailClosed(t *testing.T) {
	credential, private := testCredential(t)
	tests := []struct {
		name       string
		mutateTask func(*PrivateActionTask)
		wantError  string
	}{
		{
			name: "missing inputs",
			mutateTask: func(task *PrivateActionTask) {
				task.Inputs = nil
			},
			wantError: "signed task inputs are required",
		},
		{
			name: "command has wrong type",
			mutateTask: func(task *PrivateActionTask) {
				task.Inputs.Fields["command"] = structpb.NewNumberValue(1)
			},
			wantError: "signed task command must be a non-empty string",
		},
		{
			name: "permissions are missing",
			mutateTask: func(task *PrivateActionTask) {
				delete(task.Inputs.Fields, "effectivePermissions")
			},
			wantError: "signed task effectivePermissions is required",
		},
		{
			name: "elevatable commands have wrong type",
			mutateTask: func(task *PrivateActionTask) {
				task.Inputs.Fields["elevatableCommands"] = structpb.NewStringValue("rshell:truncate")
			},
			wantError: "signed task elevatableCommands must be an array",
		},
		{
			name: "elevatable command is empty",
			mutateTask: func(task *PrivateActionTask) {
				task.Inputs.Fields["elevatableCommands"] = structpb.NewListValue(&structpb.ListValue{Values: []*structpb.Value{structpb.NewStringValue("")}})
			},
			wantError: "signed task elevatableCommands must contain non-empty strings",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := credential.Verify(signedRequest(t, private, tc.mutateTask), time.Now())
			require.EqualError(t, err, tc.wantError)
		})
	}
}

// agentOnlyCredential returns a credential that trusts the bare request key
// and imposes no local policy.json, isolating the AgentPolicy layer.
func agentOnlyCredential(t *testing.T) (*Credential, ed25519.PrivateKey) {
	t.Helper()
	_, private, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	credential, err := NewRequestCredential([]CredentialKey{socketCredentialKey(t, private)})
	require.NoError(t, err)
	return credential, private
}

func TestVerifyWithoutAgentPolicyMatchesPreExistingBehavior(t *testing.T) {
	credential, private := testCredential(t)
	verified, err := credential.Verify(signedRequest(t, private, nil), time.Now())
	require.NoError(t, err)
	require.Equal(t, []string{"rshell:truncate"}, verified.AllowedCommands)
	require.Equal(t, []string{"/var/log"}, verified.AllowedPaths)
	require.Empty(t, verified.AllowedSystemServices)
	require.Equal(t, []string{"rshell:truncate"}, verified.ElevatableCommands)
}

// TestAgentPolicyPartialFieldsOnlyNarrowsConfiguredAxes is the regression test
// for the reported bug: an AgentPolicy that only sets AllowedCommands must not
// deny system services, paths, or elevatable commands. Those axes are left
// nil and therefore deferred entirely to signed ∩ policy.json.
func TestAgentPolicyPartialFieldsOnlyNarrowsConfiguredAxes(t *testing.T) {
	credential, private := agentOnlyCredential(t)
	req := signedRequest(t, private, func(task *PrivateActionTask) {
		task.GetSystemInputs().GetRemoteAction().SystemServices = map[string]*structpb.ListValue{
			"mysql.service": systemServiceActions(t, "read", "restart"),
		}
	})
	req.AgentPolicy = &AgentPolicy{AllowedCommands: []string{"rshell:truncate"}}

	verified, err := credential.Verify(req, time.Now())
	require.NoError(t, err)
	// Commands are narrowed by the configured axis.
	require.Equal(t, []string{"rshell:truncate"}, verified.AllowedCommands)
	// Paths, system services, and elevatable commands are untouched by the
	// agent layer because those AgentPolicy fields are nil, not empty.
	require.Equal(t, []string{"/var/log"}, verified.AllowedPaths)
	require.Equal(t, map[string][]string{"mysql.service": {"read", "restart"}}, verified.AllowedSystemServices)
	require.Equal(t, []string{"rshell:truncate"}, verified.ElevatableCommands)
}

// TestAgentPolicyExplicitEmptyFieldDeniesOnlyThatAxis mirrors the existing
// policy.json convention ("omitting allowedSystemServices denies every
// systemd action") at the per-field level: a non-nil-but-empty AgentPolicy
// field is a kill switch for that axis only, leaving nil sibling fields
// unrestricted.
func TestAgentPolicyExplicitEmptyFieldDeniesOnlyThatAxis(t *testing.T) {
	credential, private := agentOnlyCredential(t)
	req := signedRequest(t, private, func(task *PrivateActionTask) {
		task.GetSystemInputs().GetRemoteAction().SystemServices = map[string]*structpb.ListValue{
			"mysql.service": systemServiceActions(t, "read", "restart"),
		}
	})
	req.AgentPolicy = &AgentPolicy{AllowedSystemServices: map[string][]string{}}

	verified, err := credential.Verify(req, time.Now())
	require.NoError(t, err)
	// The explicit empty map denies every system service.
	require.Empty(t, verified.AllowedSystemServices)
	// Every other axis is nil on the AgentPolicy and therefore unrestricted
	// by this layer, deferring to the signed task alone.
	require.Equal(t, []string{"rshell:truncate", "rshell:echo"}, verified.AllowedCommands)
	require.Equal(t, []string{"/var/log"}, verified.AllowedPaths)
	require.Equal(t, []string{"rshell:truncate"}, verified.ElevatableCommands)
}

// TestZeroValueAgentPolicyMatchesNilAgentPolicy confirms the equivalence that
// falls out of pure per-field nil checks: a &AgentPolicy{} literal with every
// field left at its Go zero value (nil) imposes no narrowing at all, exactly
// like a nil ExecuteRequest.AgentPolicy.
func TestZeroValueAgentPolicyMatchesNilAgentPolicy(t *testing.T) {
	credential, private := testCredential(t)

	nilReq := signedRequest(t, private, nil)
	nilReq.AgentPolicy = nil
	nilVerified, err := credential.Verify(nilReq, time.Now())
	require.NoError(t, err)

	zeroReq := signedRequest(t, private, nil)
	zeroReq.AgentPolicy = &AgentPolicy{}
	zeroVerified, err := credential.Verify(zeroReq, time.Now())
	require.NoError(t, err)

	require.Equal(t, nilVerified.AllowedCommands, zeroVerified.AllowedCommands)
	require.Equal(t, nilVerified.AllowedPaths, zeroVerified.AllowedPaths)
	require.Equal(t, nilVerified.AllowedSystemServices, zeroVerified.AllowedSystemServices)
	require.Equal(t, nilVerified.ElevatableCommands, zeroVerified.ElevatableCommands)
}

// TestAgentPolicyThreeWayIntersectionEachLayerNarrowestForDifferentField
// exercises a true three-way intersection: signed, agent, and local
// policy.json are each the narrowest constraint for a different field, and
// the effective result must reflect the narrowest across all three per field.
func TestAgentPolicyThreeWayIntersectionEachLayerNarrowestForDifferentField(t *testing.T) {
	credential, private := testCredential(t)
	// Local policy.json (narrowest for system services).
	credential.AllowedCommands = []string{"rshell:*"}
	credential.AllowedPaths = []string{"/var/log"}
	credential.AllowedSystemServices = map[string][]string{"mysql.service": {"read"}}
	credential.ElevatableCommands = []string{"rshell:truncate", "rshell:systemctl"}

	req := signedRequest(t, private, func(task *PrivateActionTask) {
		// Signed task (narrowest for commands).
		task.GetSystemInputs().GetRemoteAction().AllowedCommands = []string{"rshell:truncate"}
		task.GetSystemInputs().GetRemoteAction().AllowedPaths = []string{"/var/log"}
		task.GetSystemInputs().GetRemoteAction().SystemServices = map[string]*structpb.ListValue{
			"mysql.service": systemServiceActions(t, "read", "restart", "stop"),
		}
		task.Inputs.Fields["elevatableCommands"] = structpb.NewListValue(&structpb.ListValue{
			Values: []*structpb.Value{structpb.NewStringValue("rshell:truncate"), structpb.NewStringValue("rshell:systemctl")},
		})
	})
	// Agent policy (narrowest for paths).
	req.AgentPolicy = &AgentPolicy{
		AllowedCommands:       []string{"rshell:*"},
		AllowedPaths:          []string{"/var/log/app"},
		AllowedSystemServices: map[string][]string{"mysql.service": {"read", "restart", "stop"}},
		ElevatableCommands:    []string{"rshell:truncate", "rshell:systemctl"},
	}

	verified, err := credential.Verify(req, time.Now())
	require.NoError(t, err)
	// Signed narrowest.
	require.Equal(t, []string{"rshell:truncate"}, verified.AllowedCommands)
	// Agent narrowest.
	require.Equal(t, []string{"/var/log/app"}, verified.AllowedPaths)
	// Local narrowest.
	require.Equal(t, map[string][]string{"mysql.service": {"read"}}, verified.AllowedSystemServices)
	require.ElementsMatch(t, []string{"rshell:truncate", "rshell:systemctl"}, verified.ElevatableCommands)
}

// TestAgentPolicyCannotWidenBeyondSignedTask proves an over-permissive
// AgentPolicy (e.g. the "rshell:*"/"/" wildcards) cannot expand the effective
// policy beyond what the signed task already allows.
func TestAgentPolicyCannotWidenBeyondSignedTask(t *testing.T) {
	credential, private := agentOnlyCredential(t)
	req := signedRequest(t, private, nil)
	req.AgentPolicy = &AgentPolicy{
		AllowedCommands: []string{"rshell:*"},
		AllowedPaths:    []string{"/"},
	}

	verified, err := credential.Verify(req, time.Now())
	require.NoError(t, err)
	require.Equal(t, []string{"rshell:truncate", "rshell:echo"}, verified.AllowedCommands)
	require.Equal(t, []string{"/var/log"}, verified.AllowedPaths)
}

func TestVerifyFailsClosed(t *testing.T) {
	credential, private := testCredential(t)
	tests := []struct {
		name          string
		mutateTask    func(*PrivateActionTask)
		mutateRequest func(*ExecuteRequest)
	}{
		{name: "expired", mutateTask: func(task *PrivateActionTask) { task.ExpirationTime = timestamppb.New(time.Now().Add(-time.Second)) }},
		{name: "wrong org", mutateTask: func(task *PrivateActionTask) { task.OrgId++ }},
		{name: "wrong runner", mutateTask: func(task *PrivateActionTask) { task.ConnectionInfo.RunnerId = "other" }},
		{name: "wrong action", mutateTask: func(task *PrivateActionTask) { task.ActionName = "otherAction" }},
		{name: "root mode", mutateTask: func(task *PrivateActionTask) {
			task.Inputs.Fields["effectivePermissions"] = structpb.NewStringValue("Root")
		}},
		{name: "tampered", mutateRequest: func(req *ExecuteRequest) { req.Envelope.Data = append(req.Envelope.Data, 0) }},
		{name: "unknown key", mutateRequest: func(req *ExecuteRequest) { req.Envelope.Signatures[0].KeyID = "unknown" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := signedRequest(t, private, tc.mutateTask)
			if tc.mutateRequest != nil {
				tc.mutateRequest(&req)
			}
			_, err := credential.Verify(req, time.Now())
			require.Error(t, err)
		})
	}
}
