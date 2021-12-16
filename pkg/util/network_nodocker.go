// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !docker
// +build !docker

package util

import "context"

// GetAgentNetworkMode retrieves from Docker the network mode of the Agent container
func GetAgentNetworkMode(context.Context) (string, error) {
	return "", nil
}
