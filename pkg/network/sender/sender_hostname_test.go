// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || (windows && npm)

package sender

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
)

func TestResolveHostname(t *testing.T) {
	const agentHost = "agent-host"
	const taskHost = "fargate_task:arn:aws:ecs:us-east-1:123456789012:task/cluster/0123456789abcdef"

	tests := []struct {
		name        string
		sidecar     bool
		fargateHost string
		fargateErr  error
		want        string
	}{
		{
			name: "not sidecar uses agent hostname",
			want: agentHost,
		},
		{
			name:        "sidecar uses fargate host",
			sidecar:     true,
			fargateHost: taskHost,
			want:        taskHost,
		},
		{
			name:       "sidecar falls back to agent hostname on error",
			sidecar:    true,
			fargateErr: errors.New("metadata endpoint unavailable"),
			want:       agentHost,
		},
		{
			name:    "sidecar falls back to agent hostname on empty fargate host",
			sidecar: true,
			want:    agentHost,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origIsSidecar, origGetFargateHost := isSidecar, getFargateHost
			t.Cleanup(func() { isSidecar, getFargateHost = origIsSidecar, origGetFargateHost })

			fargateCalled := false
			isSidecar = func() bool { return tt.sidecar }
			getFargateHost = func(context.Context) (string, error) {
				fargateCalled = true
				return tt.fargateHost, tt.fargateErr
			}

			hostnameComp, _ := hostname.NewMock(agentHost)
			got, err := resolveHostname(context.Background(), hostnameComp, logmock.New(t))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.sidecar, fargateCalled)
		})
	}
}
