// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package config

const (
	// AgentUser is the Agent's account on macOS, which owns the configuration files the Agent
	// reads. macOS reserves the unprefixed namespace for the operating system, so it is _dd-agent.
	// The installer's .dmg set-up uses it too: the files Fleet writes must match what it secures.
	AgentUser = "_dd-agent"
	// AgentGroup is the group of the configuration files the Agent reads, the one the installer's
	// .dmg set-up (secureConfig) gives the whole configuration tree.
	AgentGroup = "admin"

	agentConfigUser  = AgentUser
	agentConfigGroup = AgentGroup
	// agentConfigFileMode is the mode of every configuration file, the one the
	// installer's .dmg set-up (secureConfig) gives the whole configuration tree.
	agentConfigFileMode = 0660
)
