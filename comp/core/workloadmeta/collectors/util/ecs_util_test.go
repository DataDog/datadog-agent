// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package util

import (
	"testing"

	"github.com/stretchr/testify/require"
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
