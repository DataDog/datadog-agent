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

func makeConnectivityTask(inputs map[string]any) *types.Task {
	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{Inputs: inputs}
	return task
}

func TestConnectivityCheckHandlerRun(t *testing.T) {
	cases := []struct {
		name    string
		inputs  map[string]any
		wantErr string
	}{
		{
			name: "fails when inputs are malformed",
			inputs: map[string]any{
				"targetIPs": "not-a-list",
			},
			wantErr: "failed to parse connectivityCheck inputs",
		},
		{
			name: "fails when ping options are missing",
			inputs: map[string]any{
				"targetIPs": []string{"10.0.0.1"},
				"checks":    []string{"ping"},
			},
			wantErr: "failed to run connectivity checks",
		},
		{
			name: "fails when snmp options are missing",
			inputs: map[string]any{
				"targetIPs": []string{"10.0.0.1"},
				"checks":    []string{"snmp"},
			},
			wantErr: "failed to run connectivity checks",
		},
		{
			name: "fails when secret inputs cannot be decrypted",
			inputs: map[string]any{
				"targetIPs":            []string{},
				"checks":               []string{},
				"encryptedCredentials": "not-decryptable",
			},
			wantErr: "failed to decrypt secret inputs",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewConnectivityCheckHandler(encryptioncontext.NewStore())

			output, err := handler.Run(context.Background(), makeConnectivityTask(tc.inputs), nil)

			require.ErrorContains(t, err, tc.wantErr)
			require.Nil(t, output)
		})
	}
}
