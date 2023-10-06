// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !fargateprocess

package fargate

import "context"

// GetFargateHost returns the Fargate hostname used
// by the core Agent for Fargate
func GetFargateHost(ctx context.Context) (string, error) {
	return "", nil
}
