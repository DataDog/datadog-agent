// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin

package config

// Windows uses these too: config.go, which every platform builds, names them in its file specs,
// even though config_windows.go does not apply an owner or a group.
const (
	// AgentUser is the Agent's account, which owns the configuration files the Agent reads.
	AgentUser = "dd-agent"
	// AgentGroup is the group of the configuration files the Agent reads.
	AgentGroup = "dd-agent"
)
