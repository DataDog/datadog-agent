// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !ec2

package creds

import (
	"context"
	"errors"
)

// IsRunningOnAWS returns false when compiled without ec2 build tag
func IsRunningOnAWS(_ context.Context) bool {
	return false
}

// GetAWSRegion returns an error when compiled without ec2 build tag
func GetAWSRegion(_ context.Context) (string, error) {
	return "", errors.New("ec2 support not compiled in")
}
