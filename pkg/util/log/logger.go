// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

// Logger is a type implementing logging methods that create a useful interface to the log package.
type Logger struct{}

// Warnf logs with format at the warn level and returns an error containing the formated log message
func (l Logger) Warnf(format string, args ...interface{}) {
	Warnf(format, args...)
}

// Debugf logs with format at the debug level
func (l Logger) Debugf(format string, args ...interface{}) {
	Debugf(format, args...)
}
