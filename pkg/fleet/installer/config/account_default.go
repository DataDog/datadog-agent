// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !darwin

package config

const (
	// agentConfigUser owns the configuration files the Agent reads.
	agentConfigUser = "dd-agent"
	// agentConfigGroup is the group of the configuration files the Agent reads.
	agentConfigGroup = "dd-agent"
)
