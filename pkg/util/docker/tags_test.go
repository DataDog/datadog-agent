// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

// +build docker

package docker

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type MockSystemInfoClient struct {
	SwarmInfo swarm.Info
}

func (c *MockSystemInfoClient) Info(ctx context.Context) (types.Info, error) {
	return types.Info{Swarm: c.SwarmInfo}, nil
}

func TestGetTags(t *testing.T) {
	tests := []struct {
		desc       string
		mockClient SystemInfoClient
		tags       []string
	}{
		{
			"manager node with swarm active",
			&MockSystemInfoClient{SwarmInfo: swarm.Info{
				LocalNodeState:   swarm.LocalNodeStateActive,
				ControlAvailable: true,
			}},
			[]string{"docker_swarm_node_role:manager"},
		},
		{
			"worker node with swarm active",
			&MockSystemInfoClient{SwarmInfo: swarm.Info{
				LocalNodeState:   swarm.LocalNodeStateActive,
				ControlAvailable: false,
			}},
			[]string{"docker_swarm_node_role:worker"},
		},
		{
			"swarm inactive",
			&MockSystemInfoClient{SwarmInfo: swarm.Info{
				LocalNodeState:   swarm.LocalNodeStatePending,
				ControlAvailable: true,
			}},
			[]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := context.TODO()
			tags, err := getTags(tt.mockClient, ctx)
			require.NoError(t, err)
			assert.Equal(t, tt.tags, tags)
		})
	}
}
