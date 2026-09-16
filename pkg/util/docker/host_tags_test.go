// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"testing"

	"github.com/moby/moby/api/types/swarm"
	"github.com/moby/moby/api/types/system"
	"github.com/stretchr/testify/assert"
)

func TestBuildSwarmTags(t *testing.T) {
	tests := []struct {
		desc string
		info system.Info
		tags []string
	}{
		{
			"manager node with swarm active",
			system.Info{
				Swarm: swarm.Info{
					LocalNodeState:   swarm.LocalNodeStateActive,
					ControlAvailable: true,
				},
			},
			[]string{"docker_swarm_node_role:manager"},
		},
		{
			"worker node with swarm active",
			system.Info{
				Swarm: swarm.Info{
					LocalNodeState:   swarm.LocalNodeStateActive,
					ControlAvailable: false,
				},
			},
			[]string{"docker_swarm_node_role:worker"},
		},
		{
			"swarm inactive",
			system.Info{
				Swarm: swarm.Info{
					LocalNodeState:   swarm.LocalNodeStatePending,
					ControlAvailable: true,
				},
			},
			[]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			assert.Equal(t, tt.tags, buildSwarmTags(tt.info))
		})
	}
}
