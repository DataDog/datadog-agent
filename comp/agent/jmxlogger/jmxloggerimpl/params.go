// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package jmxloggerimpl

// Params defines the parameters for the JMX logger.
type Params struct {
	fromCLI  bool
	logFile  string
	disabled bool
}

// NewCliParams creates a new Params for CLI usage.
func NewCliParams(logFile string) Params {
	return Params{
		fromCLI:  true,
		logFile:  logFile,
		disabled: false,
	}
}

// NewDefaultParams creates a new Params with default values.
func NewDefaultParams() Params {
	return Params{
		fromCLI:  false,
		logFile:  "",
		disabled: false,
	}
}

// NewDisabledParams creates a new Params to disable the logger.
func NewDisabledParams() Params {
	return Params{
		fromCLI:  false,
		logFile:  "",
		disabled: true,
	}
}
