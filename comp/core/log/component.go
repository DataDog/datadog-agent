// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package log implements a component to handle logging internal to the agent.
//
// The component uses a number of values in BundleParams to decide how to
// initialize itself, reading values from the comp/core/config component when
// necessary.  At present, it configures and wraps the global logger in
// pkg/util/log, but will eventually be self-sufficient.
//
// The mock component does not read any configuration values, and redirects
// logging output to `t.Log(..)`, for ease of investigation when a test fails.
package log

// team: agent-runtimes

// Component is the component type.
type Component interface {
	// Trace logs the given arguments, separated by spaces, at the trace level
	Trace(v ...interface{})
	// Tracef logs the given formatted arguments at the trace level
	Tracef(format string, params ...interface{})

	// Debug logs the given arguments, separated by spaces, at the debug level
	Debug(v ...interface{})
	// Debugf logs the given formatted arguments at the debug level
	Debugf(format string, params ...interface{})

	// Info logs the given arguments, separated by spaces, at the info level
	Info(v ...interface{})
	// Infof logs the given formatted arguments at the info level
	Infof(format string, params ...interface{})

	// Warn logs the given arguments, separated by spaces, at the warn level,
	// and returns an error containing the messages.
	Warn(v ...interface{}) error
	// Warnf logs the given formatted arguments at the warn level, and returns
	// an error containing the message.
	Warnf(format string, params ...interface{}) error

	// Error logs the given arguments, separated by spaces, at the error level,
	// and returns an error containing the messages.
	Error(v ...interface{}) error
	// Errorf logs the given formatted arguments at the error level, and returns
	// an error containing the message.
	Errorf(format string, params ...interface{}) error

	// Critical logs the given arguments, separated by spaces, at the critical level,
	// an error containing the message.
	Critical(v ...interface{}) error
	// Criticalf logs the given formatted arguments at the critical level, and returns
	// an error containing the message.
	Criticalf(format string, params ...interface{}) error

	// Flush will flush the contents of the logs to the sinks
	Flush()
}
