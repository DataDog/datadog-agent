// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package version defines the version of the agent
package version

// AgentVersion contains the version of the Agent.
// It is populated at build time using build flags, see get_version_ldflags in tasks/utils.py
var AgentVersion string

// AgentPackageVersion contains the version of the datadog-agent package when installed by the updater.
// It has more info than AgentVersion and
// it is populated at build time using build flags, see get_version_ldflags in tasks/utils.py
var AgentPackageVersion string

// Commit is populated with the short commit hash from which the Agent was built
var Commit string

var agentVersionDefault = "6.0.0"

func init() {
	if AgentVersion == "" {
		AgentVersion = agentVersionDefault
	}
}
