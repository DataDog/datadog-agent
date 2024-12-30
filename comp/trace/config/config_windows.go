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
var defaultDDAgentBin = filepath.Join(setup.InstallPath, "bin\\agent.exe")

// defaultReceiverSocket specifies the default Unix Domain Socket to receive traces.
const defaultReceiverSocket = ""

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		DefaultLogFilePath = filepath.Join(pd, "logs", "trace-agent.log")
	}
	installDir, err := winutil.GetProgramFilesDirForProduct("DataDog Agent")
	if err != nil {
		defaultDDAgentBin = path.Join(installDir, "bin", "agent.exe")
	}
}
