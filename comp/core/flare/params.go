// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

// Params defines the parameters for the flare component.
type Params struct {
	// local is set to true when we could not contact a running Agent and the flare is created directly from the
	// CLI.
	local bool

	// distPath is the fully qualified path to the 'dist' directory
	distPath string

	// pythonChecksPath is the path to the python checks shipped with the agent
	pythonChecksPath string

	// defaultLogFile the path to the default log file
	defaultLogFile string

	// defaultJMXLogFile the path to the default JMX log file
	defaultJMXLogFile string
}

// NewLocalParams returns parameters for to initialize a local flare component. Local flares are meant to be created by
// the CLI process instead of the main Agent one.
func NewLocalParams(distPath string, pythonChecksPath string, defaultLogFile string, defaultJMXLogFile string) Params {
	p := NewParams(distPath, pythonChecksPath, defaultLogFile, defaultJMXLogFile)
	p.local = true
	return p
}

// NewLocalParams returns parameters for to initialize a non local flare component
func NewParams(distPath string, pythonChecksPath string, defaultLogFile string, defaultJMXLogFile string) Params {
	return Params{
		local:             false,
		distPath:          distPath,
		pythonChecksPath:  pythonChecksPath,
		defaultLogFile:    defaultLogFile,
		defaultJMXLogFile: defaultJMXLogFile,
	}
}
