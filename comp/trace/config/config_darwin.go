// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import "github.com/DataDog/datadog-agent/pkg/util/defaultpaths"

// team: agent-apm

// DefaultLogFilePath returns the default path where the agent will write logs
func DefaultLogFilePath() string {
	return defaultpaths.GetDefaultTraceAgentLogFile()
}

// defaultDDAgentBin specifies the default path to the main agent binary.
const defaultDDAgentBin = "/opt/datadog-agent/bin/agent/agent"
