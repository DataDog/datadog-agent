// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build docker

// Package ecs provides information about the ECS Agent Version when running in ECS
package ecs

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetRegionAndAWSAccountID(t *testing.T) {
	tests := []struct {
		name            string
		arn             string
		expectedRegion  string
		expectedAccount string
	}{
		{
			name:            "standard aws partition",
			arn:             "arn:aws:ecs:us-east-1:123427279990:container-instance/ecs-my-cluster/123412345abcdefgh34999999",
			expectedRegion:  "us-east-1",
			expectedAccount: "123427279990",
		},
		{
			name:            "aws-us-gov partition",
			arn:             "arn:aws-us-gov:ecs:us-gov-west-1:123456789012:container-instance/ecs-gov-cluster/abcdef123456",
			expectedRegion:  "us-gov-west-1",
			expectedAccount: "123456789012",
		},
		{
			name:            "aws-cn partition",
			arn:             "arn:aws-cn:ecs:cn-north-1:987654321098:container-instance/ecs-china-cluster/xyz789",
			expectedRegion:  "cn-north-1",
			expectedAccount: "987654321098",
		},
		{
			name:            "invalid partition returns empty",
			arn:             "arn:aws-invalid:ecs:us-east-1:123456789012:container-instance/cluster/id",
			expectedRegion:  "",
			expectedAccount: "",
		},
		{
			name:            "malformed arn returns empty",
			arn:             "not-an-arn",
			expectedRegion:  "",
			expectedAccount: "",
		},
		{
			name:            "invalid account id length returns empty account",
			arn:             "arn:aws:ecs:us-east-1:123:task/12345678-1234-1234-1234-123456789012",
			expectedRegion:  "us-east-1",
			expectedAccount: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			region, id := ParseRegionAndAWSAccountID(tt.arn)
			require.Equal(t, tt.expectedRegion, region)
			require.Equal(t, tt.expectedAccount, id)
		})
	}
}

func TestInitClusterID(t *testing.T) {
	id1, err := initClusterID("123456789012", "us-east-1", "ecs-cluster-1")
	require.NoError(t, err)
	require.Equal(t, "34616234-6562-3536-3733-656534636532", id1)

	// same account, same region, different cluster name
	id2, err := initClusterID("123456789012", "us-east-1", "ecs-cluster-2")
	require.NoError(t, err)
	require.Equal(t, "31643131-3131-3263-3331-383136383336", id2)

	// same account, different region, same cluster name
	id3, err := initClusterID("123456789012", "us-east-2", "ecs-cluster-1")
	require.NoError(t, err)
	require.Equal(t, "64663464-6662-3232-3635-646166613230", id3)

	// different account, same region, same cluster name
	id4, err := initClusterID("123456789013", "us-east-1", "ecs-cluster-1")
	require.NoError(t, err)
	require.Equal(t, "61623431-6137-6231-3136-366464643761", id4)
}

// TestNewECSMetaTaskMetadataFallback covers the fallback from the v1 introspection endpoint
// to the agent's own task metadata. The v1 endpoint is not reachable on ECS Managed
// Instances, but the v4 task payload carries the same cluster identity.
func TestNewECSMetaTaskMetadataFallback(t *testing.T) {
	instanceErr := errors.New("temporary failure in ecsutil-meta-v1")
	taskErr := errors.New("v4 metadata endpoint not available")

	instanceMeta := func(context.Context) (string, string, string, string, error) {
		return "123456789012", "us-east-1", "ecs-cluster-1", "Amazon ECS Agent - v1.54.0", nil
	}
	taskMeta := func(context.Context) (string, string, string, string, error) {
		return "123456789012", "us-east-1", "ecs-cluster-1", "3", nil
	}
	failing := func(err error) func(context.Context) (string, string, string, string, error) {
		return func(context.Context) (string, string, string, string, error) {
			return "", "", "", "", err
		}
	}

	tests := []struct {
		name         string
		instanceFunc func(context.Context) (string, string, string, string, error)
		taskFunc     func(context.Context) (string, string, string, string, error)
		expectErr    error
		expectMeta   *MetaECS
	}{
		{
			name:         "instance metadata available is used directly",
			instanceFunc: instanceMeta,
			taskFunc:     failing(taskErr),
			expectMeta: &MetaECS{
				AWSAccountID:    "123456789012",
				Region:          "us-east-1",
				ECSCluster:      "ecs-cluster-1",
				ECSClusterID:    "34616234-6562-3536-3733-656534636532",
				ECSAgentVersion: "Amazon ECS Agent - v1.54.0",
			},
		},
		{
			name:         "instance metadata failure falls back to task metadata",
			instanceFunc: failing(instanceErr),
			taskFunc:     taskMeta,
			expectMeta: &MetaECS{
				AWSAccountID: "123456789012",
				Region:       "us-east-1",
				ECSCluster:   "ecs-cluster-1",
				// Identical to the instance-metadata cluster ID: initClusterID is derived
				// purely from account, region and cluster name.
				ECSClusterID:    "34616234-6562-3536-3733-656534636532",
				ECSAgentVersion: "3",
			},
		},
		{
			name:         "both unavailable surfaces the original instance metadata error",
			instanceFunc: failing(instanceErr),
			taskFunc:     failing(taskErr),
			expectErr:    instanceErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origInstance, origTask := fetchECSInstanceMetadata, fetchECSTaskMetadata
			t.Cleanup(func() {
				fetchECSInstanceMetadata, fetchECSTaskMetadata = origInstance, origTask
			})
			fetchECSInstanceMetadata, fetchECSTaskMetadata = tt.instanceFunc, tt.taskFunc

			meta, err := newECSMeta(context.Background())

			if tt.expectErr != nil {
				require.ErrorIs(t, err, tt.expectErr)
				require.Nil(t, meta)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.expectMeta, meta)
		})
	}
}

func TestMetaECS_toCacheValue(t *testing.T) {
	tests := []struct {
		name     string
		meta     MetaECS
		expected string
	}{
		{
			name: "complete metadata",
			meta: MetaECS{
				AWSAccountID:    "123456789012",
				Region:          "us-east-1",
				ECSCluster:      "my-cluster",
				ECSClusterID:    "cluster-id-123",
				ECSAgentVersion: "1.2.3",
			},
			expected: "123456789012:us-east-1:my-cluster:cluster-id-123:1.2.3",
		},
		{
			name: "metadata with empty values",
			meta: MetaECS{
				AWSAccountID:    "",
				Region:          "",
				ECSCluster:      "",
				ECSClusterID:    "",
				ECSAgentVersion: "",
			},
			expected: "::::",
		},
		{
			name: "metadata with special characters",
			meta: MetaECS{
				AWSAccountID:    "123456789012",
				Region:          "us-west-2",
				ECSCluster:      "cluster-with-dashes",
				ECSClusterID:    "id-with-uuid-format",
				ECSAgentVersion: "1.2.3-beta",
			},
			expected: "123456789012:us-west-2:cluster-with-dashes:id-with-uuid-format:1.2.3-beta",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.meta.toCacheValue()
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestMetaECS_fromCacheValue(t *testing.T) {
	tests := []struct {
		name        string
		cacheValue  string
		expected    MetaECS
		expectError bool
	}{
		{
			name:       "valid cache value",
			cacheValue: "123456789012:us-east-1:my-cluster:cluster-id-123:1.2.3",
			expected: MetaECS{
				AWSAccountID:    "123456789012",
				Region:          "us-east-1",
				ECSCluster:      "my-cluster",
				ECSClusterID:    "cluster-id-123",
				ECSAgentVersion: "1.2.3",
			},
			expectError: false,
		},
		{
			name:       "cache value with empty fields",
			cacheValue: "::::",
			expected: MetaECS{
				AWSAccountID:    "",
				Region:          "",
				ECSCluster:      "",
				ECSClusterID:    "",
				ECSAgentVersion: "",
			},
			expectError: false,
		},
		{
			name:        "invalid cache value - too few parts",
			cacheValue:  "123456789012:us-east-1:my-cluster",
			expected:    MetaECS{},
			expectError: true,
		},
		{
			name:        "invalid cache value - too many parts",
			cacheValue:  "123456789012:us-east-1:my-cluster:cluster-id:1.2.3:extra",
			expected:    MetaECS{},
			expectError: true,
		},
		{
			name:        "invalid cache value - empty string",
			cacheValue:  "",
			expected:    MetaECS{},
			expectError: true,
		},
		{
			name:        "invalid cache value - no colons",
			cacheValue:  "invalid",
			expected:    MetaECS{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := &MetaECS{}
			err := meta.fromCacheValue(tt.cacheValue)

			if tt.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), "invalid cache value")
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected.AWSAccountID, meta.AWSAccountID)
				require.Equal(t, tt.expected.Region, meta.Region)
				require.Equal(t, tt.expected.ECSCluster, meta.ECSCluster)
				require.Equal(t, tt.expected.ECSClusterID, meta.ECSClusterID)
				require.Equal(t, tt.expected.ECSAgentVersion, meta.ECSAgentVersion)
			}
		})
	}
}

func TestMetaECS_RoundTrip(t *testing.T) {
	tests := []struct {
		name string
		meta MetaECS
	}{
		{
			name: "complete metadata round trip",
			meta: MetaECS{
				AWSAccountID:    "123456789012",
				Region:          "us-east-1",
				ECSCluster:      "production-cluster",
				ECSClusterID:    "abc-def-123-456",
				ECSAgentVersion: "1.50.0",
			},
		},
		{
			name: "metadata with hyphens and dots",
			meta: MetaECS{
				AWSAccountID:    "987654321098",
				Region:          "eu-central-1",
				ECSCluster:      "test-cluster-name",
				ECSClusterID:    "uuid-format-id",
				ECSAgentVersion: "2.0.0-rc1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Convert to cache value
			cacheValue := tt.meta.toCacheValue()

			// Convert back from cache value
			meta := &MetaECS{}
			err := meta.fromCacheValue(cacheValue)

			require.NoError(t, err)
			require.Equal(t, tt.meta.AWSAccountID, meta.AWSAccountID)
			require.Equal(t, tt.meta.Region, meta.Region)
			require.Equal(t, tt.meta.ECSCluster, meta.ECSCluster)
			require.Equal(t, tt.meta.ECSClusterID, meta.ECSClusterID)
			require.Equal(t, tt.meta.ECSAgentVersion, meta.ECSAgentVersion)
		})
	}
}
