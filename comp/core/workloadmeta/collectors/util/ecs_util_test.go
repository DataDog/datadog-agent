// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/ecs/metadata/v3or4"
)

func TestParserECSAgentVersion(t *testing.T) {
	for _, testCase := range []struct {
		version  string
		expected string
	}{
		{
			version:  "Amazon ECS Agent - v1.30.0 (02ff320c)",
			expected: "1.30.0",
		},
		{
			version:  "some prefix v1.30.0-beta some suffix",
			expected: "1.30.0-beta",
		},
		{
			version:  "some prefix v1 some suffix",
			expected: "1",
		},
		{
			version:  "some prefix v0.1 some suffix",
			expected: "0.1",
		},
		{
			version:  "Amazon ECS Agent - (02ff320c)",
			expected: "",
		},
		{
			version:  "someprefixv0.1somesuffix",
			expected: "",
		},
	} {
		version := ParseECSAgentVersion(testCase.version)
		require.Equal(t, testCase.expected, version)
	}
}

func TestBuildClusterARN(t *testing.T) {
	arn := BuildClusterARN("cluster-name", "123456789012", "us-east-1")
	require.Equal(t, "arn:aws:ecs:us-east-1:123456789012:cluster/cluster-name", arn)
}

func TestBuildServiceARN(t *testing.T) {
	arn := BuildServiceARN("cluster-name", "service-name", "123456789012", "us-east-1")
	require.Equal(t, "arn:aws:ecs:us-east-1:123456789012:service/cluster-name/service-name", arn)
}

func TestBuildTaskDefinitionARN(t *testing.T) {
	arn := BuildTaskDefinitionARN("123456789012", "family-name", "us-east-1", "1")
	require.Equal(t, "arn:aws:ecs:us-east-1:123456789012:task-definition/family-name:1", arn)
}

func newMinimalTask(launchType string) v3or4.Task {
	return v3or4.Task{
		TaskARN:     "arn:aws:ecs:us-east-1:123456789012:task/cluster/abc123",
		Family:      "test-family",
		Version:     "1",
		ClusterName: "cluster",
		LaunchType:  launchType,
	}
}

func getECSTaskEntity(t *testing.T, events []workloadmeta.CollectorEvent) *workloadmeta.ECSTask {
	t.Helper()
	for _, e := range events {
		if task, ok := e.Entity.(*workloadmeta.ECSTask); ok {
			return task
		}
	}
	t.Fatal("no ECSTask entity found in events")
	return nil
}

func TestParseV4TaskLaunchTypeEC2(t *testing.T) {
	events := ParseV4Task(newMinimalTask("EC2"), map[workloadmeta.EntityID]struct{}{})
	task := getECSTaskEntity(t, events)
	assert.Equal(t, workloadmeta.ECSLaunchTypeEC2, task.LaunchType)
}

func TestParseV4TaskLaunchTypeManagedInstances(t *testing.T) {
	// MANAGED_INSTANCES should be set regardless of sidecar/daemon mode
	events := ParseV4Task(newMinimalTask("MANAGED_INSTANCES"), map[workloadmeta.EntityID]struct{}{})
	task := getECSTaskEntity(t, events)
	assert.Equal(t, workloadmeta.ECSLaunchTypeManagedInstances, task.LaunchType)
}

func TestParseV4TaskLaunchTypeManagedInstancesCaseInsensitive(t *testing.T) {
	events := ParseV4Task(newMinimalTask("managed_instances"), map[workloadmeta.EntityID]struct{}{})
	task := getECSTaskEntity(t, events)
	assert.Equal(t, workloadmeta.ECSLaunchTypeManagedInstances, task.LaunchType)
}

func TestParseV4TaskLaunchTypeFargate(t *testing.T) {
	events := ParseV4Task(newMinimalTask("FARGATE"), map[workloadmeta.EntityID]struct{}{})
	task := getECSTaskEntity(t, events)
	assert.Equal(t, workloadmeta.ECSLaunchTypeFargate, task.LaunchType)
}
