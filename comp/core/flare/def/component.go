// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flare implements a component to generate flares from the agent.
//
// A flare is a archive containing all the information necessary to troubleshoot the Agent. When opening a support
// ticket a flare might be requested. Flares contain the Agent logs, configurations and much more.
package flare

import (
	"time"

	"github.com/DataDog/datadog-agent/comp/core/flare/types"
)

// team: agent-configuration

// Component is the component type.
type Component interface {
	// Create creates a new flare locally and returns the path to the flare file.
	//
	// If providerTimeout is 0 or negative, the timeout from the configuration will be used.
	Create(pdata types.ProfileData, providerTimeout time.Duration, ipcError error, diagnoseResult []byte) (string, error)
	// CreateWithArgs creates a new flare locally and returns the path to the flare file.
	// This function is used to create a flare with specific arguments.
	CreateWithArgs(flareArgs types.FlareArgs, providerTimeout time.Duration, ipcError error, diagnoseResult []byte) (string, error)
	// Send sends a flare archive to Datadog. The local archive is removed after a successful upload unless the component was created with KeepArchiveAfterSend (e.g. CLI --keep-archive).
	Send(flarePath string, caseID string, email string, source types.FlareSource) (string, error)
}

// Params defines the parameters for the flare component.
type Params struct {
	// local is set to true when we could not contact a running Agent and the flare is created directly from the
	// CLI.
	Local bool

	// KeepArchiveAfterSend when true keeps the local flare archive file after a successful upload (e.g. for CLI --keep-archive).
	KeepArchiveAfterSend bool

	// DistPath is the fully qualified path to the 'dist' directory
	DistPath string

	// PythonChecksPath is the path to the python checks shipped with the agent
	PythonChecksPath string

	// DefaultLogFile the path to the default log file
	DefaultLogFile string

	// DefaultJMXLogFile the path to the default JMX log file
	DefaultJMXLogFile string

	// DefaultDogstatsdLogFile the path to the default dogstatsd log file
	DefaultDogstatsdLogFile string

	// DefaultStreamlogsLogFile the path to the default Streamlogs log file
	DefaultStreamlogsLogFile string
}

// NewLocalParams returns parameters to initialize a local flare component. Local flares are meant to be created by
// the CLI process instead of the main Agent one.
func NewLocalParams(distPath string, pythonChecksPath string, defaultLogFile string, defaultJMXLogFile string, defaultDogstatsdLogFile string, defaultStreamlogsLogFile string) Params {
	p := NewParams(distPath, pythonChecksPath, defaultLogFile, defaultJMXLogFile, defaultDogstatsdLogFile, defaultStreamlogsLogFile)
	p.Local = true
	return p
}

// NewParams returns parameters to initialize a non local flare component.
func NewParams(distPath string, pythonChecksPath string, defaultLogFile string, defaultJMXLogFile string, defaultDogstatsdLogFile string, defaultStreamlogsLogFile string) Params {
	return Params{
		Local:                    false,
		DistPath:                 distPath,
		PythonChecksPath:         pythonChecksPath,
		DefaultLogFile:           defaultLogFile,
		DefaultJMXLogFile:        defaultJMXLogFile,
		DefaultDogstatsdLogFile:  defaultDogstatsdLogFile,
		DefaultStreamlogsLogFile: defaultStreamlogsLogFile,
	}
}
