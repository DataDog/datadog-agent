// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

// Params defines the parameters for the flare component.
type Params struct {
	// Local is set to true when we could not contact a running Agent and the flare is created directly from the
	// CLI.
	Local bool

	// DistPath is the fully qualified path to the 'dist' directory
	DistPath string

	// PythonChecksPath is the path to the python checks shipped with the agent
	PythonChecksPath string

	// DefaultLogFile the path to the default log file
	DefaultLogFile string

	// DefaultJMXLogFile the path to the default JMX log file
	DefaultJMXLogFile string

	// DefaultDogstatsdLogFile the path to the default JMX log file
	DefaultDogstatsdLogFile string
}

// NewLocalParams returns parameters for to initialize a local flare component. Local flares are meant to be created by
// the CLI process instead of the main Agent one.
func NewLocalParams(distPath string, pythonChecksPath string, defaultLogFile string, defaultJMXLogFile string, defaultDogstatsdLogFile string) Params {
	p := NewParams(distPath, pythonChecksPath, defaultLogFile, defaultJMXLogFile, defaultDogstatsdLogFile)
	p.Local = true
	return p
}

// NewParams returns parameters for to initialize a non local flare component
func NewParams(distPath string, pythonChecksPath string, defaultLogFile string, defaultJMXLogFile string, defaultDogstatsdLogFile string) Params {
	return Params{
		Local:                   false,
		DistPath:                distPath,
		PythonChecksPath:        pythonChecksPath,
		DefaultLogFile:          defaultLogFile,
		DefaultJMXLogFile:       defaultJMXLogFile,
		DefaultDogstatsdLogFile: defaultDogstatsdLogFile,
	}
}
