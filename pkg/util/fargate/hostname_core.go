// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build !fargateprocess

package fargate

// GetFargateHost returns the Fargate hostname used
// by the core Agent for Fargate
func GetFargateHost() (string, error) {
	return "", nil
}
