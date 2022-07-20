// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This module should be updated at every release

package version

import "github.com/DataDog/datadog-agent/pkg/traceinit"

// AgentVersion contains the version of the Agent
var AgentVersion string

// Commit is populated with the short commit hash from which the Agent was built
var Commit string

var agentVersionDefault = "6.0.0"

func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\version\base.go 18`)
	if AgentVersion == "" {
		AgentVersion = agentVersionDefault
	}
	traceinit.TraceFunction(`\DataDog\datadog-agent\pkg\version\base.go 21`)
}