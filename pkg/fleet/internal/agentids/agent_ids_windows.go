// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package agentids provides a method to get the agent user UID/GID.
package agentids

// GetAgentIDs returns the UID and GID of the dd-agent user and group.
func GetAgentIDs() (_, _ int, _ error) {
	return -1, -1, nil
}
