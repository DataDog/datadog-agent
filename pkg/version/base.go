// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This module should be updated at every release

package version

// AgentVersion contains the version of the Agent
var AgentVersion string

// Commit is populated with the short commit hash from which the Agent was built
var Commit string

var agentVersionDefault = "6.0.0"

func init() {
	if AgentVersion == "" {
		AgentVersion = agentVersionDefault
	}
}
