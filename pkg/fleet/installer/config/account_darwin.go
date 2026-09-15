// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package config

const (
	// agentConfigUser owns the configuration files the Agent reads. macOS reserves the
	// unprefixed namespace for the operating system, so the Agent's account is _dd-agent.
	agentConfigUser = "_dd-agent"
	// agentConfigGroup is the group of the configuration files the Agent reads. macOS has no
	// per-user group, so the Agent's account belongs to the system daemon group.
	agentConfigGroup = "daemon"
)
