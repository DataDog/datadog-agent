// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package haagent

// Role type for HA Agent Role
type Role string

const (
	// Leader HA Agent role
	Leader Role = "leader"
	// Follower HA Agent role
	Follower Role = "follower"
)
