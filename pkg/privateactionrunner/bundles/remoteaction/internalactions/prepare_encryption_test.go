// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_internal

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/box"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func newTask(taskID string, inputs map[string]any) *types.Task {
	task := &types.Task{}
	task.Data.ID = taskID
	task.Data.Attributes = &types.Attributes{
		BundleID: "com.datadoghq.remoteaction.internal",
		Name:     "prepareEncryption",
		Inputs:   inputs,
	}
	return task
}

func assertSealRoundTrip(t *testing.T, store encryptioncontext.Store, result *PrepareEncryptionOutputs) {
	t.Helper()

	publicKey, err := base64.StdEncoding.DecodeString(result.PublicKey)
	require.NoError(t, err)
	require.Len(t, publicKey, 32, "Curve25519 public key must be 32 bytes")

	privateKey, err := store.Take(result.EncryptionContextID)
	require.NoError(t, err)
	require.NotNil(t, privateKey)

	var publicKeyArray [32]byte
	copy(publicKeyArray[:], publicKey)
	plaintext := []byte("hello")
	sealed, err := box.SealAnonymous(nil, plaintext, &publicKeyArray, nil)
	require.NoError(t, err)
	opened, ok := box.OpenAnonymous(nil, sealed, &publicKeyArray, privateKey)
	require.True(t, ok)
	require.Equal(t, plaintext, opened)
}

func TestPrepareEncryptionRun(t *testing.T) {
	cases := []struct {
		name    string
		inputs  map[string]any
		wantErr bool
	}{
		{
			name:   "generates key pair, populates output and stores private key",
			inputs: map[string]any{"encryptionContextId": "ctx-abc"},
		},
		{
			name:    "rejects missing encryptionContextId",
			inputs:  map[string]any{},
			wantErr: true,
		},
		{
			name:    "rejects empty encryptionContextId",
			inputs:  map[string]any{"encryptionContextId": ""},
			wantErr: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			store := encryptioncontext.NewStore(time.Now)
			handler := NewPrepareEncryptionHandler(store)

			output, err := handler.Run(context.Background(), newTask("task-abc", testCase.inputs), nil)
			if testCase.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			result, ok := output.(*PrepareEncryptionOutputs)
			require.True(t, ok, "unexpected output type %T", output)
			require.Equal(t, "curve25519", result.KeyType)
			require.Equal(t, testCase.inputs["encryptionContextId"], result.EncryptionContextID)
			assertSealRoundTrip(t, store, result)
		})
	}
}

func TestPrepareEncryptionGeneratesUniqueContextsPerRun(t *testing.T) {
	store := encryptioncontext.NewStore(time.Now)
	handler := NewPrepareEncryptionHandler(store)

	runs := []string{"first", "second"}
	results := make([]*PrepareEncryptionOutputs, 0, len(runs))
	for _, name := range runs {
		t.Run(name, func(_ *testing.T) {
			output, err := handler.Run(context.Background(), newTask("task", map[string]any{"encryptionContextId": name}), nil)
			require.NoError(t, err)
			result, ok := output.(*PrepareEncryptionOutputs)
			require.True(t, ok)
			results = append(results, result)
		})
	}

	require.NotEqual(t, results[0].EncryptionContextID, results[1].EncryptionContextID)
	require.NotEqual(t, results[0].PublicKey, results[1].PublicKey)
}

func TestInternalBundleGetAction(t *testing.T) {
	cases := []struct {
		name        string
		actionName  string
		wantPresent bool
	}{
		{name: "known action", actionName: "prepareEncryption", wantPresent: true},
		{name: "unknown action", actionName: "doesNotExist", wantPresent: false},
	}

	store := encryptioncontext.NewStore(time.Now)
	bundle := NewInternal(store)
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			action := bundle.GetAction(testCase.actionName)
			if testCase.wantPresent {
				require.NotNil(t, action)
			} else {
				require.Nil(t, action)
			}
		})
	}
}
