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
	region, awsAccountID := parseRegionAndAWSAccountID(arn)
	require.Equal(t, "us-east-1", region)
	require.Equal(t, 123456789012, awsAccountID)

	// test invalid arn
	arn = "arn:aws:ecs:us-east-1:123:task/12345678-1234-1234-1234-123456789012"
	region, awsAccountID = parseRegionAndAWSAccountID(arn)
	require.Equal(t, "us-east-1", region)
	require.Equal(t, 0, awsAccountID)
}
