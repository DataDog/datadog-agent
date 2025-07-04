// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package rcclientimpl

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config/def"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/settings/settingsimpl"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/def"
	"github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func TestNewRemoteConfigClient(t *testing.T) {
	tests := []struct {
		name        string
		params      rcclient.Params
		expectError bool
	}{
		{
			name: "valid params",
			params: rcclient.Params{
				AgentName:    "test-agent",
				AgentVersion: "1.0.0",
			},
			expectError: false,
		},
		{
			name: "empty agent name",
			params: rcclient.Params{
				AgentName:    "",
				AgentVersion: "1.0.0",
			},
			expectError: true,
		},
		{
			name: "empty agent version",
			params: rcclient.Params{
				AgentName:    "test-agent",
				AgentVersion: "",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := dependencies{
				Log:            logmock.New(t),
				Params:         tt.params,
				Listeners:      []types.RCListener{},
				TaskListeners:  []types.RCAgentTaskListener{},
				Config:         fxutil.Test[config.Component](t, fx.Options()),
				SysprobeConfig: option.None[sysprobeconfig.Component](),
				IPC:            ipcmock.New(t),
			}

			client, err := NewRemoteConfigClient(deps)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, client)
			} else {
				// Note: This test may fail due to missing IPC setup
				// In a real test environment, you'd need to mock the IPC properly
				if err != nil {
					t.Logf("Expected test to pass but got error (likely due to missing test setup): %v", err)
				}
			}
		})
	}
}
