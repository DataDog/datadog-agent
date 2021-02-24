// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import log "github.com/cihub/seelog"

// Logger interface used to remove the dependency of this package to the logger of the agent
type Logger interface {
	// Tracef is used to print a trace level log
	Tracef(format string, params ...interface{})
	// Debugf is used to print a trace level log
	Debugf(format string, params ...interface{})
	// Errorf is used to print an error
	Errorf(format string, params ...interface{}) error
}

// DefaultLogger is a wrapper for the agent logger that we use to prevent a dependency on packages that we cannot
// import outside of the agent repository
type DefaultLogger struct{}

// Tracef is used to print a trace level log
func (l DefaultLogger) Tracef(format string, params ...interface{}) {
	log.Tracef(format, params)
}

// Debugf is used to print a trace level log
func (l DefaultLogger) Debugf(format string, params ...interface{}) {
	log.Debugf(format, params)
}

// Errorf is used to print an error
func (l DefaultLogger) Errorf(format string, params ...interface{}) error {
	return log.Errorf(format, params)
}
