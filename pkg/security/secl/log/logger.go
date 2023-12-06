// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package log holds log related files
package log

// Logger interface used to remove the dependency of this package to the logger of the agent
type Logger interface {
	// Infof is used to print a info level log
	Infof(format string, params ...interface{})
	// Tracef is used to print a trace level log
	Tracef(format string, params ...interface{})
	// Debugf is used to print a trace level log
	Debugf(format string, params ...interface{})
	// Errorf is used to print an error
	Errorf(format string, params ...interface{})

	IsTracing() bool
}

// NullLogger is a default implementation of the Logger interface
type NullLogger struct{}

// Tracef is used to print a trace level log
func (l NullLogger) Tracef(_ string, _ ...interface{}) {
}

// Debugf is used to print a trace level log
func (l NullLogger) Debugf(_ string, _ ...interface{}) {
}

// Errorf is used to print an error
func (l NullLogger) Errorf(_ string, _ ...interface{}) {
}

// Infof is used to print an info
func (l NullLogger) Infof(_ string, _ ...interface{}) {
}

// IsTracing is used to check if TraceF would actually log
func (l NullLogger) IsTracing() bool {
	return false
}

// OrNullLogger ensures that the provided logger is non-nil by returning a NullLogger if it is
func OrNullLogger(potential Logger) Logger {
	if potential != nil {
		return potential
	}
	return &NullLogger{}
}
