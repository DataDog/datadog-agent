// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsStatusReady(t *testing.T) {
	tests := []struct {
		name          string
		payload       string
		expectedReady bool
	}{
		{
			name:          "Invalid config",
			payload:       "Error: unable to load Datadog config file: While parsing config: yaml: line 2: mapping values are not allowed in this context",
			expectedReady: false,
		},
		{
			name: "Not ready",
			payload: `Getting the status from the agent.

Could not reach agent: Get "https://localhost:5001/agent/status": dial tcp 127.0.0.1:5001: connect: connection refused
Make sure the agent is running before requesting the status and contact support if you continue having issues.
Error: Get "https://localhost:5001/agent/status": dial tcp 127.0.0.1:5001: connect: connection refused`,
			expectedReady: false,
		},
		{
			name: "Ready status",
			payload: `Getting the status from the agent.


===============
Agent (v7.44.0)
===============

	Status date: 2023-05-02 16:42:35.338 UTC (1683045755338)
	Agent start: 2023-05-02 16:42:25.942 UTC (1683045745942)
	Pid: 7440
	Go Version: go1.19.7
	Python Version: 3.8.16
	Build arch: amd64
	Agent flavor: agent
	Check Runners: 4
	Log Level: info`,
			expectedReady: true,
		},
		{
			name: "Ready with rc",
			payload: `Getting the status from the agent.


===============
Agent (v7.44.0-rc-7)
===============

	Status date: 2023-05-02 16:42:35.338 UTC (1683045755338)
	Agent start: 2023-05-02 16:42:25.942 UTC (1683045745942)
	Pid: 7440
	Go Version: go1.19.7
	Python Version: 3.8.16
	Build arch: amd64
	Agent flavor: agent
	Check Runners: 4
	Log Level: info`,
			expectedReady: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStatus(tt.payload)

			isReady, err := s.IsReady()
			require.NoError(t, err)
			assert.Equal(t, tt.expectedReady, isReady)
		})
	}

}
