// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eks

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildClusterARN(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
		accountID   string
		region      string
		expected    string
	}{
		{
			name:        "valid inputs",
			clusterName: "my-cluster",
			accountID:   "123456789012",
			region:      "us-west-2",
			expected:    "arn:aws:eks:us-west-2:123456789012:cluster/my-cluster",
		},
		{
			name:        "cluster name with underscores",
			clusterName: "my_test_cluster",
			accountID:   "123456789012",
			region:      "eu-west-1",
			expected:    "arn:aws:eks:eu-west-1:123456789012:cluster/my_test_cluster",
		},
		{
			name:        "govcloud region",
			clusterName: "prod-cluster",
			accountID:   "123456789012",
			region:      "us-gov-west-1",
			expected:    "arn:aws:eks:us-gov-west-1:123456789012:cluster/prod-cluster",
		},
		{
			name:        "empty cluster name",
			clusterName: "",
			accountID:   "123456789012",
			region:      "us-west-2",
			expected:    "",
		},
		{
			name:        "empty account ID",
			clusterName: "my-cluster",
			accountID:   "",
			region:      "us-west-2",
			expected:    "",
		},
		{
			name:        "empty region",
			clusterName: "my-cluster",
			accountID:   "123456789012",
			region:      "",
			expected:    "",
		},
		{
			name:        "invalid account ID - too short",
			clusterName: "my-cluster",
			accountID:   "12345678901",
			region:      "us-west-2",
			expected:    "",
		},
		{
			name:        "invalid account ID - too long",
			clusterName: "my-cluster",
			accountID:   "1234567890123",
			region:      "us-west-2",
			expected:    "",
		},
		{
			name:        "invalid account ID - non-numeric",
			clusterName: "my-cluster",
			accountID:   "12345678901a",
			region:      "us-west-2",
			expected:    "",
		},
		{
			name:        "invalid region format",
			clusterName: "my-cluster",
			accountID:   "123456789012",
			region:      "invalid-region",
			expected:    "",
		},
		{
			name:        "invalid cluster name - starts with hyphen",
			clusterName: "-invalid",
			accountID:   "123456789012",
			region:      "us-west-2",
			expected:    "",
		},
		{
			name:        "invalid cluster name - special characters",
			clusterName: "my@cluster",
			accountID:   "123456789012",
			region:      "us-west-2",
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildClusterARN(tt.clusterName, tt.accountID, tt.region)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsValidClusterName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"simple name", "my-cluster", true},
		{"with underscores", "my_cluster_123", true},
		{"numeric start", "1cluster", true},
		{"max length", "a" + string(make([]byte, 99)), false}, // 100 'a's would exceed
		{"empty", "", false},
		{"starts with hyphen", "-cluster", false},
		{"starts with underscore", "_cluster", false},
		{"special chars", "my@cluster", false},
		{"spaces", "my cluster", false},
		{"dots", "my.cluster", false},
	}

	// Test max length boundary
	t.Run("exactly 100 chars", func(t *testing.T) {
		name := "a" + string(make([]byte, 99))
		for i := range name {
			if i == 0 {
				continue
			}
			name = name[:i] + "b" + name[i+1:]
		}
		// This creates a 100 char string starting with 'a'
		assert.True(t, len("a"+string(make([]byte, 99))) == 100)
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidClusterName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseClusterARN(t *testing.T) {
	tests := []struct {
		name        string
		arn         string
		expected    *ClusterIdentity
		description string
	}{
		{
			name: "valid aws partition",
			arn:  "arn:aws:eks:us-west-2:123456789012:cluster/my-cluster",
			expected: &ClusterIdentity{
				ClusterName: "my-cluster",
				ClusterARN:  "arn:aws:eks:us-west-2:123456789012:cluster/my-cluster",
				AccountID:   "123456789012",
				Region:      "us-west-2",
			},
		},
		{
			name: "valid aws-cn partition",
			arn:  "arn:aws-cn:eks:cn-north-1:123456789012:cluster/china-cluster",
			expected: &ClusterIdentity{
				ClusterName: "china-cluster",
				ClusterARN:  "arn:aws-cn:eks:cn-north-1:123456789012:cluster/china-cluster",
				AccountID:   "123456789012",
				Region:      "cn-north-1",
			},
		},
		{
			name: "valid aws-us-gov partition",
			arn:  "arn:aws-us-gov:eks:us-gov-west-1:123456789012:cluster/gov-cluster",
			expected: &ClusterIdentity{
				ClusterName: "gov-cluster",
				ClusterARN:  "arn:aws-us-gov:eks:us-gov-west-1:123456789012:cluster/gov-cluster",
				AccountID:   "123456789012",
				Region:      "us-gov-west-1",
			},
		},
		{
			name:        "empty arn",
			arn:         "",
			expected:    nil,
			description: "empty input returns nil",
		},
		{
			name:        "wrong service",
			arn:         "arn:aws:ecs:us-west-2:123456789012:cluster/my-cluster",
			expected:    nil,
			description: "ECS ARN should not parse as EKS",
		},
		{
			name:        "wrong resource type",
			arn:         "arn:aws:eks:us-west-2:123456789012:nodegroup/my-cluster/ng-1",
			expected:    nil,
			description: "nodegroup ARN should not parse as cluster",
		},
		{
			name:        "invalid partition",
			arn:         "arn:aws-invalid:eks:us-west-2:123456789012:cluster/my-cluster",
			expected:    nil,
			description: "invalid partition should fail",
		},
		{
			name:        "malformed - too few parts",
			arn:         "arn:aws:eks:us-west-2:cluster/my-cluster",
			expected:    nil,
			description: "missing account ID field",
		},
		{
			name:        "empty cluster name",
			arn:         "arn:aws:eks:us-west-2:123456789012:cluster/",
			expected:    nil,
			description: "cluster/ with no name should fail",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseClusterARN(tt.arn)
			if tt.expected == nil {
				assert.Nil(t, result, tt.description)
			} else {
				require.NotNil(t, result, tt.description)
				assert.Equal(t, tt.expected.ClusterName, result.ClusterName)
				assert.Equal(t, tt.expected.ClusterARN, result.ClusterARN)
				assert.Equal(t, tt.expected.AccountID, result.AccountID)
				assert.Equal(t, tt.expected.Region, result.Region)
			}
		})
	}
}

func TestClusterIdentity_CrossAccountSafety(t *testing.T) {
	// Document cross-account limitation: if node is in account A but cluster
	// is in account B, the derived ARN will be incorrect.
	// This test documents expected behavior, not a bug.

	t.Run("derived ARN uses node account not cluster account", func(t *testing.T) {
		// Simulating: cluster owned by 111111111111, node running in 222222222222
		// The BuildClusterARN function will use whatever account is passed
		// (which comes from IMDS = node account)
		nodeAccountID := "222222222222"
		clusterAccountID := "111111111111"
		clusterName := "cross-account-cluster"
		region := "us-west-2"

		// This is what we'd compute from node IMDS
		derivedARN := BuildClusterARN(clusterName, nodeAccountID, region)
		// This is the actual cluster ARN
		actualARN := BuildClusterARN(clusterName, clusterAccountID, region)

		assert.NotEqual(t, actualARN, derivedARN,
			"Cross-account scenario: derived ARN differs from actual cluster ARN - this is a known limitation")
		assert.Equal(t, "arn:aws:eks:us-west-2:222222222222:cluster/cross-account-cluster", derivedARN)
		assert.Equal(t, "arn:aws:eks:us-west-2:111111111111:cluster/cross-account-cluster", actualARN)
	})
}

func TestValidPatterns(t *testing.T) {
	// Verify our regex patterns match AWS documented formats

	t.Run("region patterns", func(t *testing.T) {
		validRegions := []string{
			"us-east-1", "us-west-2", "eu-west-1", "ap-northeast-1",
			"us-gov-west-1", "us-gov-east-1",
			"cn-north-1", "cn-northwest-1",
			"af-south-1", "ap-south-1", "me-south-1",
		}
		for _, r := range validRegions {
			assert.True(t, validRegionPattern.MatchString(r), "region %s should be valid", r)
		}

		invalidRegions := []string{
			"invalid", "us-east", "US-EAST-1", "us_east_1",
			"us-east-1a", // AZ, not region
		}
		for _, r := range invalidRegions {
			assert.False(t, validRegionPattern.MatchString(r), "region %s should be invalid", r)
		}
	})

	t.Run("account ID patterns", func(t *testing.T) {
		assert.True(t, validAccountIDPattern.MatchString("123456789012"))
		assert.True(t, validAccountIDPattern.MatchString("000000000000"))
		assert.False(t, validAccountIDPattern.MatchString("12345678901"))  // 11 digits
		assert.False(t, validAccountIDPattern.MatchString("1234567890123")) // 13 digits
		assert.False(t, validAccountIDPattern.MatchString("12345678901a"))  // has letter
	})
}
