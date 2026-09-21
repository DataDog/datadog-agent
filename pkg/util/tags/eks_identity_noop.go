// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build !ec2

// Package tags provides utilities for working with tags.
package tags

import "context"

// GetEKSClusterIdentityTags is a no-op stub for non-EC2 builds.
// EKS identity resolution requires IMDS which is only available on EC2.
func GetEKSClusterIdentityTags(_ context.Context, _ string) []string {
	return nil
}
