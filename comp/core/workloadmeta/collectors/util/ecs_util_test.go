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

func TestParseRegionAndAWSAccountID(t *testing.T) {
	// test valid arn
	arn := "arn:aws:ecs:us-east-1:123456789012:task/12345678-1234-1234-1234-123456789012"
	region, awsAccountID := ParseRegionAndAWSAccountID(arn)
	require.Equal(t, "us-east-1", region)
	require.Equal(t, 123456789012, awsAccountID)

	// test invalid arn
	arn = "arn:aws:ecs:us-east-1:123:task/12345678-1234-1234-1234-123456789012"
	region, awsAccountID = ParseRegionAndAWSAccountID(arn)
	require.Equal(t, "us-east-1", region)
	require.Equal(t, 0, awsAccountID)
}

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
