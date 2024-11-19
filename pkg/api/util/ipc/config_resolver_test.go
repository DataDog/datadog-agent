// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ipc

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestGetConfigAddrResolver(t *testing.T) {
	cfg := configmock.New(t)
	tests := []struct {
		name       string
		endpoint   string
		configFunc func()
		expected   string
		expectErr  bool
	}{
		{
			name:     "CoreCmd",
			endpoint: CoreCmd,
			configFunc: func() {
				cfg.SetWithoutSource("cmd_host", "127.0.0.1")
				cfg.SetWithoutSource("process_config.cmd_port", "5001")
			},
			expected:  "127.0.0.1:5001",
			expectErr: false,
		},
		{
			name:     "CoreIPC",
			endpoint: CoreIPC,
			configFunc: func() {
				cfg.SetWithoutSource("agent_ipc.host", "127.0.0.1")
				cfg.SetWithoutSource("agent_ipc.port", "5002")

			},
			expected:  "127.0.0.1:5002",
			expectErr: false,
		},
		{
			name:     "CoreExpvar",
			endpoint: CoreExpvar,
			configFunc: func() {
				cfg.SetWithoutSource("cmd_host", "127.0.0.1")
				cfg.SetWithoutSource("expvar_port", "5003")

			},
			expected:  "127.0.0.1:5003",
			expectErr: false,
		},
		{
			name:     "TraceCmd",
			endpoint: TraceCmd,
			configFunc: func() {
				cfg.SetWithoutSource("cmd_host", "127.0.0.1")
				cfg.SetWithoutSource("apm_config.debug.port", "5004")

			},
			expected:  "127.0.0.1:5004",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.configFunc()
			resolver := NewConfigResolver()

			endoints, err := resolver.Resolve(tt.endpoint)

			assert.NoError(t, err)

			assert.NotNil(t, endoints)
			assert.Len(t, endoints, 1)

			assert.Equal(t, tt.expected, endoints[0].Addr())
		})
	}
}
