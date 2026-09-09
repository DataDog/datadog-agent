// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_queries

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

const testFingerprint = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func resolveTaskWithInputs(inputs map[string]interface{}) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		BundleID: BundleID,
		Name:     ResolveActionName,
		Inputs:   inputs,
	}
	return task
}

// TestBundleRegistersResolveAction proves the resolve action is registered in the
// PAR bundle registry alongside execute under its own action name.
func TestBundleRegistersResolveAction(t *testing.T) {
	bundle := NewRemoteQueriesBundle()

	require.NotNil(t, bundle.GetAction(ResolveActionName))
	require.NotNil(t, bundle.GetAction(ExecuteActionName))
	assert.Equal(t, "resolve", ResolveActionName)
}

// TestResolveActionUsesCredentialFreeAgentSecureRequestShape proves the resolve
// request carries exactly the integration and target: no query, no result
// delivery, and no fingerprint echo, and the private credentials never reach
// the AgentSecure request.
func TestResolveActionUsesCredentialFreeAgentSecureRequestShape(t *testing.T) {
	client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{
		Status:           "matched",
		MatchFingerprint: testFingerprint,
	}}
	action := NewResolveAction(func() (BridgeClient, error) { return client, nil })

	output, err := action.Run(context.Background(), resolveTaskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
	}), &privateconnection.PrivateCredentials{Tokens: []privateconnection.PrivateCredentialsToken{{Name: "password", Value: "secret-value"}}})

	require.NoError(t, err)
	require.NotNil(t, client.resolveRequest)
	assert.Equal(t, "postgres", client.resolveRequest.GetIntegration())
	assert.Equal(t, "localhost", client.resolveRequest.GetTarget().GetHost())
	assert.Equal(t, int32(5432), client.resolveRequest.GetTarget().GetPort())
	assert.Equal(t, "postgres", client.resolveRequest.GetTarget().GetDbname())
	assert.Empty(t, client.resolveRequest.GetTarget().GetDatabaseInstance())

	requestEvidence, err := json.Marshal(client.resolveRequest)
	require.NoError(t, err)
	assert.NotContains(t, string(requestEvidence), "secret-value")

	out, ok := output.(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, map[string]interface{}{
		"status":           "matched",
		"matchFingerprint": testFingerprint,
	}, out)
}

// TestResolveActionAcceptsDatabaseInstanceTarget proves the managed-instance
// selector maps through the resolve request like execute's target mapping.
func TestResolveActionAcceptsDatabaseInstanceTarget(t *testing.T) {
	client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{
		Status:           "matched",
		MatchFingerprint: testFingerprint,
	}}
	action := NewResolveAction(func() (BridgeClient, error) { return client, nil })

	_, err := action.Run(context.Background(), resolveTaskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"target":      map[string]interface{}{"database_instance": "Rq-Proof-A1-DB1"},
	}), nil)

	require.NoError(t, err)
	require.NotNil(t, client.resolveRequest)
	assert.Equal(t, "Rq-Proof-A1-DB1", client.resolveRequest.GetTarget().GetDatabaseInstance())
	assert.Empty(t, client.resolveRequest.GetTarget().GetHost())
}

// TestResolveActionRejectsExecuteOnlyFieldsBeforeRPC proves the resolve action is
// side-effect-free by construction: a resolve input carrying any execute-only
// field — query, includeSchema, resultDelivery, or even a fingerprint — never
// creates the bridge client.
func TestResolveActionRejectsExecuteOnlyFieldsBeforeRPC(t *testing.T) {
	executeOnlyInputs := []struct {
		name  string
		extra map[string]interface{}
	}{
		{
			name:  "query",
			extra: map[string]interface{}{"query": "SELECT secret FROM private_table"},
		},
		{
			name:  "includeSchema",
			extra: map[string]interface{}{"includeSchema": true},
		},
		{
			name:  "resultDelivery",
			extra: map[string]interface{}{"resultDelivery": resultDeliveryInputs()},
		},
		{
			name:  "matchFingerprint",
			extra: map[string]interface{}{"matchFingerprint": testFingerprint},
		},
	}

	for _, tt := range executeOnlyInputs {
		t.Run(tt.name, func(t *testing.T) {
			action := NewResolveAction(func() (BridgeClient, error) {
				require.Fail(t, "bridge client should not be created for a resolve input with execute-only fields")
				return nil, nil
			})

			inputs := map[string]interface{}{
				"integration": "postgres",
				"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
			}
			for key, value := range tt.extra {
				inputs[key] = value
			}

			_, err := action.Run(context.Background(), resolveTaskWithInputs(inputs), nil)

			require.Error(t, err)
			var parErr util.PARError
			require.ErrorAs(t, err, &parErr)
			assert.Equal(t, "invalid remote query action inputs", parErr.Message)
			assert.NotContains(t, err.Error(), "SELECT secret")
			assert.NotContains(t, err.Error(), testToken)
			assert.NotContains(t, err.Error(), testFingerprint)
		})
	}
}

// TestResolveActionRejectsInvalidTargetBeforeRPC mirrors execute's target
// validation: malformed selectors never create the bridge client.
func TestResolveActionRejectsInvalidTargetBeforeRPC(t *testing.T) {
	tests := []struct {
		name   string
		target map[string]interface{}
	}{
		{name: "mixed selectors", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "host": "localhost", "port": 5432, "dbname": "postgres"}},
		{name: "partial tuple", target: map[string]interface{}{"host": "localhost", "dbname": "postgres"}},
		{name: "whitespace instance", target: map[string]interface{}{"database_instance": " rq-proof-a1-db1 "}},
		{name: "unknown credential field", target: map[string]interface{}{"database_instance": "rq-proof-a1-db1", "password": "secret-value"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			action := NewResolveAction(func() (BridgeClient, error) {
				require.Fail(t, "bridge client should not be created for an invalid resolve target")
				return nil, nil
			})

			_, err := action.Run(context.Background(), resolveTaskWithInputs(map[string]interface{}{
				"integration": "postgres",
				"target":      tt.target,
			}), nil)

			require.Error(t, err)
			var parErr util.PARError
			require.ErrorAs(t, err, &parErr)
			assert.Equal(t, "invalid remote query action inputs", parErr.Message)
			assert.NotContains(t, err.Error(), "secret-value")
		})
	}
}

// TestResolveActionPropagatesStructuredOutcomes proves the typed resolve response
// maps to the AP output envelope: non-matched statuses carry the error object
// mirroring the status, matched carries the fingerprint.
func TestResolveActionPropagatesStructuredOutcomes(t *testing.T) {
	t.Run("target not found", func(t *testing.T) {
		client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{
			Status:       "target_not_found",
			ErrorCode:    "target_not_found",
			ErrorMessage: "no matching integration check found",
		}}
		action := NewResolveAction(func() (BridgeClient, error) { return client, nil })

		output, err := action.Run(context.Background(), resolveTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "secret-db"},
		}), nil)

		require.NoError(t, err)
		assert.Equal(t, map[string]interface{}{
			"status": "target_not_found",
			"error": map[string]interface{}{
				"code":    "target_not_found",
				"message": "no matching integration check found",
			},
		}, output)
	})

	t.Run("ambiguous", func(t *testing.T) {
		client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{
			Status:       "ambiguous_target",
			ErrorCode:    "ambiguous_target",
			ErrorMessage: "multiple matching integration checks found",
		}}
		action := NewResolveAction(func() (BridgeClient, error) { return client, nil })

		output, err := action.Run(context.Background(), resolveTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"database_instance": "duplicate"},
		}), nil)

		require.NoError(t, err)
		assert.Equal(t, map[string]interface{}{
			"status": "ambiguous_target",
			"error": map[string]interface{}{
				"code":    "ambiguous_target",
				"message": "multiple matching integration checks found",
			},
		}, output)
	})

	t.Run("resolution error", func(t *testing.T) {
		client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{
			Status:       "resolution_error",
			ErrorCode:    "resolution_error",
			ErrorMessage: "remote queries resolve bridge is disabled",
		}}
		action := NewResolveAction(func() (BridgeClient, error) { return client, nil })

		output, err := action.Run(context.Background(), resolveTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		}), nil)

		require.NoError(t, err)
		assert.Equal(t, map[string]interface{}{
			"status": "resolution_error",
			"error": map[string]interface{}{
				"code":    "resolution_error",
				"message": "remote queries resolve bridge is disabled",
			},
		}, output)
	})
}

// TestResolveActionPassesThroughWellFormedContractViolations proves the bundle
// maps responses opaquely: a matched response without a fingerprint and an
// unknown status pass through unchanged so the dispatcher classifies them
// (resolution_error) instead of the bundle collapsing them into a transport
// failure.
func TestResolveActionPassesThroughWellFormedContractViolations(t *testing.T) {
	t.Run("matched without fingerprint", func(t *testing.T) {
		client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{Status: "matched"}}
		action := NewResolveAction(func() (BridgeClient, error) { return client, nil })

		output, err := action.Run(context.Background(), resolveTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		}), nil)

		require.NoError(t, err)
		assert.Equal(t, map[string]interface{}{"status": "matched"}, output)
	})

	t.Run("unknown status", func(t *testing.T) {
		client := &captureBridgeClient{resolveResp: &pb.RemoteQueryResolveResponse{Status: "something_unexpected"}}
		action := NewResolveAction(func() (BridgeClient, error) { return client, nil })

		output, err := action.Run(context.Background(), resolveTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		}), nil)

		require.NoError(t, err)
		assert.Equal(t, map[string]interface{}{"status": "something_unexpected"}, output)
	})
}

// TestResolveActionFailsClosedOnMalformedResponses proves structurally invalid
// responses — a nil response or a missing status — surface as PAR action errors
// with sanitized display messages.
func TestResolveActionFailsClosedOnMalformedResponses(t *testing.T) {
	tests := []struct {
		name         string
		resp         *pb.RemoteQueryResolveResponse
		wantExternal string
	}{
		{name: "nil response", resp: nil, wantExternal: "remote query AgentSecure resolve RPC response was invalid"},
		{name: "missing status", resp: &pb.RemoteQueryResolveResponse{}, wantExternal: "remote query AgentSecure resolve RPC response was invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &captureBridgeClient{resolveResp: tt.resp}
			action := NewResolveAction(func() (BridgeClient, error) { return client, nil })

			_, err := action.Run(context.Background(), resolveTaskWithInputs(map[string]interface{}{
				"integration": "postgres",
				"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
			}), nil)

			require.Error(t, err)
			var parErr util.PARError
			require.ErrorAs(t, err, &parErr)
			assert.Equal(t, tt.wantExternal, parErr.ExternalMessage)
		})
	}
}

// TestResolveActionSurfacesTransportFailure proves an AgentSecure transport
// failure on the resolve RPC surfaces as a sanitized PAR action error.
func TestResolveActionSurfacesTransportFailure(t *testing.T) {
	client := &captureBridgeClient{err: assert.AnError}
	action := NewResolveAction(func() (BridgeClient, error) { return client, nil })

	_, err := action.Run(context.Background(), resolveTaskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
	}), nil)

	require.Error(t, err)
	var parErr util.PARError
	require.ErrorAs(t, err, &parErr)
	assert.Equal(t, "remote query AgentSecure resolve RPC failed", parErr.ExternalMessage)
}

// TestResolveActionRequiresBridgeClient mirrors execute's client guards.
func TestResolveActionRequiresBridgeClient(t *testing.T) {
	t.Run("missing factory", func(t *testing.T) {
		var action *ResolveAction

		_, err := action.Run(context.Background(), resolveTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		}), nil)

		require.Error(t, err)
		var parErr util.PARError
		require.ErrorAs(t, err, &parErr)
		assert.Equal(t, "remote query action requires an Agent IPC client", parErr.Message)
	})

	t.Run("client creation failure", func(t *testing.T) {
		action := NewResolveAction(func() (BridgeClient, error) { return nil, assert.AnError })

		_, err := action.Run(context.Background(), resolveTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		}), nil)

		require.Error(t, err)
		var parErr util.PARError
		require.ErrorAs(t, err, &parErr)
		assert.Equal(t, "remote query action could not create an Agent IPC client", parErr.ExternalMessage)
	})

	t.Run("nil client", func(t *testing.T) {
		action := NewResolveAction(func() (BridgeClient, error) { return nil, nil })

		_, err := action.Run(context.Background(), resolveTaskWithInputs(map[string]interface{}{
			"integration": "postgres",
			"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "postgres"},
		}), nil)

		require.Error(t, err)
		var parErr util.PARError
		require.ErrorAs(t, err, &parErr)
		assert.Equal(t, "remote query action requires an AgentSecure client", parErr.Message)
	})
}

// TestResolveActionSanitizesInputExtractionErrors proves a malformed resolve task
// input fails closed before the bridge client with a constant, sanitized error.
func TestResolveActionSanitizesInputExtractionErrors(t *testing.T) {
	action := NewResolveAction(func() (BridgeClient, error) {
		require.Fail(t, "bridge client should not be created for invalid inputs")
		return nil, nil
	})

	_, err := action.Run(context.Background(), resolveTaskWithInputs(map[string]interface{}{
		"integration": "postgres",
		"target":      map[string]interface{}{"host": "localhost", "port": 5432, "dbname": "secret-db"},
		"bad":         make(chan struct{}),
	}), nil)

	require.Error(t, err)
	var parErr util.PARError
	require.ErrorAs(t, err, &parErr)
	assert.Equal(t, "invalid remote query action inputs", parErr.Message)
	assert.Equal(t, "invalid remote query action inputs", parErr.ExternalMessage)
	assert.NotContains(t, err.Error(), "secret-db")
}
