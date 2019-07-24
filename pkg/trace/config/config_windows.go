// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package config

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/pkg/util/executable"
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

var (
	// DefaultLogFilePath is where the agent will write logs if not overridden in the conf
	DefaultLogFilePath = "c:\\programdata\\datadog\\logs\\trace-agent.log"

	// Agent 5 Python Environment - exposes access to Python utilities
	// such as obtaining the hostname from GCE, EC2, Kube, etc.
	defaultDDAgentPy    = "c:\\Program Files\\Datadog\\Datadog Agent\\embedded\\python.exe"
	defaultDDAgentPyEnv = "PYTHONPATH=c:\\Program Files\\Datadog\\Datadog Agent\\agent"

	// Agent 6
	defaultDDAgentBin = "c:\\Program Files\\Datadog\\Datadog Agent\\bin\\agent.exe"
)

// agent5Config points to the default agent 5 configuration path. It is used
// as a fallback when no configuration is set and the new default is missing.
const agent5Config = "c:\\programdata\\datadog\\datadog.conf"

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
