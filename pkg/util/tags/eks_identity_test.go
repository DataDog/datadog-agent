// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build ec2

package tags

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetEKSClusterIdentityTags_EmptyClusterName(t *testing.T) {
	// Empty cluster name should return nil without attempting resolution
	tags := GetEKSClusterIdentityTags(context.Background(), "")
	assert.Nil(t, tags)
}

func TestGetEKSClusterIdentityTags_InvalidClusterName(t *testing.T) {
	// Invalid cluster names should fail validation and return nil
	// Names starting with numbers or containing invalid characters
	tags := GetEKSClusterIdentityTags(context.Background(), "123-invalid")
	assert.Nil(t, tags)

	tags = GetEKSClusterIdentityTags(context.Background(), "-starts-with-hyphen")
	assert.Nil(t, tags)
}

func TestGetEKSClusterIdentityTags_ContextCancelled(t *testing.T) {
	// Cancelled context should return nil (fail closed)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	tags := GetEKSClusterIdentityTags(ctx, "valid-cluster-name")
	assert.Nil(t, tags)
}
