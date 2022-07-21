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
import "github.com/DataDog/datadog-agent/pkg/traceinit"

// DefaultLogFilePath is where the agent will write logs if not overridden in the conf
var DefaultLogFilePath = "c:\\programdata\\datadog\\logs\\trace-agent.log"

// defaultDDAgentBin specifies the default path to the main agent executable.
var defaultDDAgentBin = "c:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe"

func init() {
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\trace-agent\config\config_windows.go 21`)
	pd, err := winutil.GetProgramDataDir()
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\trace-agent\config\config_windows.go 22`)
	if err == nil {
		DefaultLogFilePath = filepath.Join(pd, "logs", "trace-agent.log")
	}
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\trace-agent\config\config_windows.go 25`)
	_here, err := executable.Folder()
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\trace-agent\config\config_windows.go 26`)
	if err == nil {
		defaultDDAgentBin = filepath.Join(_here, "..", "agent.exe")
	}
	traceinit.TraceFunction(`\DataDog\datadog-agent\cmd\trace-agent\config\config_windows.go 29`)

}
