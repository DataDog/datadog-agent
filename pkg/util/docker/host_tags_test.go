// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"context"
	"testing"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/docker/fake"
)

func TestGetTags(t *testing.T) {
	tests := []struct {
		desc   string
		client client.SystemAPIClient
		tags   []string
	}{
		{
			"manager node with swarm active",
			&fake.SystemAPIClient{
				InfoFunc: func() (types.Info, error) {
					return types.Info{
						Swarm: swarm.Info{
							LocalNodeState:   swarm.LocalNodeStateActive,
							ControlAvailable: true,
						},
					}, nil
				},
			},
			[]string{"docker_swarm_node_role:manager"},
		},
		{
			"worker node with swarm active",
			&fake.SystemAPIClient{
				InfoFunc: func() (types.Info, error) {
					return types.Info{
						Swarm: swarm.Info{
							LocalNodeState:   swarm.LocalNodeStateActive,
							ControlAvailable: false,
						},
					}, nil
				},
			},
			[]string{"docker_swarm_node_role:worker"},
		},
		{
			"swarm inactive",
			&fake.SystemAPIClient{
				InfoFunc: func() (types.Info, error) {
					return types.Info{
						Swarm: swarm.Info{
							LocalNodeState:   swarm.LocalNodeStatePending,
							ControlAvailable: true,
						},
					}, nil
				},
			},
			[]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			ctx := context.TODO()
			tags, err := getTags(ctx, tt.client)
			require.NoError(t, err)
			assert.Equal(t, tt.tags, tags)
		})
	}
}
