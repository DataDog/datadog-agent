// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

// team: agent-apm

// DefaultLogFilePath is where the agent will write logs if not overridden in the conf
var DefaultLogFilePath = "c:\\programdata\\datadog\\logs\\trace-agent.log"

// defaultDDAgentBin specifies the default path to the main agent executable.
var defaultDDAgentBin = "c:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe"

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		DefaultLogFilePath = filepath.Join(pd, "logs", "trace-agent.log")
	}
	_here, err := executable.Folder()
	if err == nil {
		defaultDDAgentBin = filepath.Join(_here, "..", "agent.exe")
	}

}
