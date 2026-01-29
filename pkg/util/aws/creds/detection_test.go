// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ec2

package creds

import (
	"context"
	"testing"
)

func TestIsRunningOnAWS(_ *testing.T) {
	// This test just ensures the function is callable and doesn't panic
	// The actual behavior depends on the environment
	ctx := context.Background()
	_ = IsRunningOnAWS(ctx)
}

func TestGetAWSRegion(_ *testing.T) {
	// This test just ensures the function is callable and doesn't panic
	// The actual behavior depends on the environment
	ctx := context.Background()
	_, _ = GetAWSRegion(ctx)
}
