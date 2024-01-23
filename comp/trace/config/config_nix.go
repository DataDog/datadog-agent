// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !darwin

package config

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
)

// team: agent-apm

// DefaultLogFilePath is where the agent will write logs if not overridden in the conf
const DefaultLogFilePath = "/var/log/datadog/trace-agent.log"

// defaultDDAgentBin specifies the default path to the main agent binary.
var defaultDDAgentBin = filepath.Join(setup.InstallPath, "bin/agent/agent")
