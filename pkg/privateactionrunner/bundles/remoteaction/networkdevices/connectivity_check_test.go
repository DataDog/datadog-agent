// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_remoteaction_networkdevices

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

func TestConnectivityCheckHandlerRun(t *testing.T) {
	cases := []struct {
		name            string
		taskInputs      map[string]any
		expectedResults interface{}
		expectedErr     string
	}{
		{
			name: "fails when inputs are malformed",
			taskInputs: map[string]any{
				"targetIPs": "not-a-list",
			},
			expectedErr: "failed to parse connectivityCheck inputs",
		},
		{
			name: "fails when ping options are missing",
			taskInputs: map[string]any{
				"targetIPs": []string{"10.0.0.1"},
				"checks":    []string{"ping"},
			},
			expectedErr: "failed to run connectivity checks",
		},
		{
			name: "fails when snmp options are missing",
			taskInputs: map[string]any{
				"targetIPs": []string{"10.0.0.1"},
				"checks":    []string{"snmp"},
			},
			expectedErr: "failed to run connectivity checks",
		},
		{
			name: "fails when secret inputs cannot be decrypted",
			taskInputs: map[string]any{
				"targetIPs":            []string{},
				"checks":               []string{},
				"encryptedCredentials": "not-decryptable",
			},
			expectedErr: "failed to decrypt secret inputs",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewConnectivityCheckHandler(encryptioncontext.NewStore())

			res, err := handler.Run(context.Background(), makeConnectivityTask(tc.taskInputs), nil)
			require.Equal(t, tc.expectedResults, res)
			require.ErrorContains(t, err, tc.expectedErr)
		})
	}
}

func makeConnectivityTask(taskInputs map[string]any) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{Inputs: taskInputs}
	return task
}
