// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build kubeapiserver,orchestrator

package cluster

import (
	"testing"

	model "github.com/DataDog/agent-payload/process"
	"github.com/stretchr/testify/assert"
)

func TestChunkDeployments(t *testing.T) {
	deploys := []*model.Deployment{
		{
			Metadata: &model.Metadata{
				Uid: "1",
			},
		},
		{
			Metadata: &model.Metadata{
				Uid: "2",
			},
		},
		{
			Metadata: &model.Metadata{
				Uid: "3",
			},
		},
		{
			Metadata: &model.Metadata{
				Uid: "4",
			},
		},
		{
			Metadata: &model.Metadata{
				Uid: "5",
			},
		},
	}
	expected := [][]*model.Deployment{
		{{
			Metadata: &model.Metadata{
				Uid: "1",
			},
		},
			{
				Metadata: &model.Metadata{
					Uid: "2",
				},
			}},
		{{
			Metadata: &model.Metadata{
				Uid: "3",
			},
		},
			{
				Metadata: &model.Metadata{
					Uid: "4",
				},
			}},
		{{
			Metadata: &model.Metadata{
				Uid: "5",
			},
		}},
	}
	actual := chunkDeployments(deploys, 3, 2)
	assert.ElementsMatch(t, expected, actual)
}

func TestChunkReplicasets(t *testing.T) {
	rs := []*model.ReplicaSet{
		{
			Metadata: &model.Metadata{
				Uid: "1",
			},
		},
		{
			Metadata: &model.Metadata{
				Uid: "2",
			},
		},
		{
			Metadata: &model.Metadata{
				Uid: "3",
			},
		},
		{
			Metadata: &model.Metadata{
				Uid: "4",
			},
		},
		{
			Metadata: &model.Metadata{
				Uid: "5",
			},
		},
	}
	expected := [][]*model.ReplicaSet{
		{{
			Metadata: &model.Metadata{
				Uid: "1",
			},
		},
			{
				Metadata: &model.Metadata{
					Uid: "2",
				},
			}},
		{{
			Metadata: &model.Metadata{
				Uid: "3",
			},
		},
			{
				Metadata: &model.Metadata{
					Uid: "4",
				},
			}},
		{{
			Metadata: &model.Metadata{
				Uid: "5",
			},
		}},
	}
	actual := chunkReplicaSets(rs, 3, 2)
	assert.ElementsMatch(t, expected, actual)
}

func TestConvertNodeStatusToTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "Ready,SchedulingDisabled",
			input:    "Ready,SchedulingDisabled",
			expected: []string{"node_status:ready", "node_schedulable:false"},
		}, {
			name:     "Ready",
			input:    "Ready",
			expected: []string{"node_status:ready", "node_schedulable:true"},
		}, {
			name:     "Unknown",
			input:    "Unknown",
			expected: []string{"node_status:unknown", "node_schedulable:true"},
		}, {
			name:     "",
			input:    "",
			expected: []string{"node_schedulable:true"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, convertNodeStatusToTags(tt.input))
		})
	}
}
