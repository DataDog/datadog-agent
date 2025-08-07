// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ec2

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSecurityGroups(t *testing.T) {
	ctx := context.Background()

	// This test will only work on actual EC2 instances
	// On non-EC2 environments, it should return an error
	securityGroups, err := GetSecurityGroups(ctx)

	if err != nil {
		// Expected on non-EC2 environments
		t.Logf("GetSecurityGroups returned error (expected on non-EC2): %v", err)
		return
	}

	// If we're on EC2, we should get some security groups
	require.NotNil(t, securityGroups)
	t.Logf("Found security groups: %v", securityGroups)

	// Security groups should not be empty if we're on EC2
	assert.Greater(t, len(securityGroups), 0, "Should have at least one security group on EC2")

	// Each security group should be a valid format (sg-xxxxxxxxx)
	for _, sg := range securityGroups {
		assert.Contains(t, sg, "sg-", "Security group should start with 'sg-'")
		assert.Greater(t, len(sg), 3, "Security group ID should be longer than 'sg-'")
	}
}

func TestGetSecurityGroupsForInterface(t *testing.T) {
	ctx := context.Background()

	// This test will only work on actual EC2 instances
	// On non-EC2 environments, it should return an error
	securityGroups, err := GetSecurityGroupsForInterface(ctx)

	if err != nil {
		// Expected on non-EC2 environments
		t.Logf("GetSecurityGroupsForInterface returned error (expected on non-EC2): %v", err)
		return
	}

	// If we're on EC2, we should get some security groups
	require.NotNil(t, securityGroups)
	t.Logf("Found security groups for interfaces: %v", securityGroups)

	// Security groups should not be empty if we're on EC2
	assert.Greater(t, len(securityGroups), 0, "Should have at least one security group on EC2")

	// Each security group should be a valid format (sg-xxxxxxxxx)
	for _, sg := range securityGroups {
		assert.Contains(t, sg, "sg-", "Security group should start with 'sg-'")
		assert.Greater(t, len(sg), 3, "Security group ID should be longer than 'sg-'")
	}
}

func TestGetSecurityGroupsConsistency(t *testing.T) {
	ctx := context.Background()

	// Test that both methods return consistent results
	sg1, err1 := GetSecurityGroups(ctx)
	sg2, err2 := GetSecurityGroupsForInterface(ctx)

	// If both methods fail, that's expected on non-EC2
	if err1 != nil && err2 != nil {
		t.Logf("Both methods failed (expected on non-EC2): %v, %v", err1, err2)
		return
	}

	// If one succeeds and the other doesn't, that's interesting but not necessarily wrong
	if err1 != nil {
		t.Logf("GetSecurityGroups failed: %v", err1)
	}
	if err2 != nil {
		t.Logf("GetSecurityGroupsForInterface failed: %v", err2)
	}

	// If both succeed, they should return the same security groups
	if err1 == nil && err2 == nil {
		assert.ElementsMatch(t, sg1, sg2, "Both methods should return the same security groups")
	}
}
