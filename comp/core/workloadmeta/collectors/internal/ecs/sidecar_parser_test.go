// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package ecs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

// TestHandleUnseenEntities tests the handleUnseenEntities method
func TestHandleUnseenEntities(t *testing.T) {
	collector := &collector{
		seen: map[workloadmeta.EntityID]struct{}{
			{Kind: workloadmeta.KindECSTask, ID: "old-task-id"}:     {},
			{Kind: workloadmeta.KindContainer, ID: "old-container"}: {},
		},
	}

	currentSeen := map[workloadmeta.EntityID]struct{}{
		{Kind: workloadmeta.KindECSTask, ID: "new-task-id"}: {},
	}

	events := []workloadmeta.CollectorEvent{}
	events = collector.handleUnseenEntities(events, currentSeen, workloadmeta.SourceRuntime)

	// Should generate 2 unset events for the old task and container
	require.Len(t, events, 2)
	for _, event := range events {
		assert.Equal(t, workloadmeta.EventTypeUnset, event.Type)
		assert.Equal(t, workloadmeta.SourceRuntime, event.Source)
	}
}

// TestParseClusterName tests the parseClusterName method
func TestParseClusterName(t *testing.T) {
	collector := &collector{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple name",
			input:    "my-cluster",
			expected: "my-cluster",
		},
		{
			name:     "ARN format",
			input:    "arn:aws:ecs:us-east-1:123456789:cluster/my-cluster",
			expected: "my-cluster",
		},
		{
			name:     "ARN with multiple slashes",
			input:    "arn:aws:ecs:us-east-1:123456789:cluster/prod/my-cluster",
			expected: "my-cluster",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := collector.parseClusterName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestParseStatus tests the parseStatus method
func TestParseStatus(t *testing.T) {
	collector := &collector{}

	tests := []struct {
		input    string
		expected workloadmeta.ContainerStatus
	}{
		{"RUNNING", workloadmeta.ContainerStatusRunning},
		{"STOPPED", workloadmeta.ContainerStatusStopped},
		{"PULLED", workloadmeta.ContainerStatusCreated},
		{"CREATED", workloadmeta.ContainerStatusCreated},
		{"RESOURCES_PROVISIONED", workloadmeta.ContainerStatusCreated},
		{"UNKNOWN_STATUS", workloadmeta.ContainerStatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := collector.parseStatus(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
