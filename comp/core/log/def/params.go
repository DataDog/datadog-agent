// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"os"
	"runtime"
)

// Params defines the parameters for this log component.
//
// Logs-related parameters are implemented as unexported fields containing
// callbacks.  These fields can be set with the `LogXxx()` methods, which
// return the updated LogParams.  One of `logimpl.ForOneShot` or `logimpl.ForDaemon`
// must be called.
type Params struct {
	// loggerName is the name that appears in the logfile
	loggerName string

	// logLevelFn returns the log level. This field is set by methods on this
	// type.
	logLevelFn func(configGetter) string

	// logFileFn returns the log file. This field is set by methods on this type.
	logFileFn func(configGetter) string

	// logSyslogURIFn returns the syslog URI. This field is set by methods on this type.
	logSyslogURIFn func(configGetter) string

	// logSyslogRFCFn returns a boolean determining whether to use syslog RFC
	// 5424. This field is set by methods on this type.
	logSyslogRFCFn func(configGetter) bool

	// logToConsoleFn returns a boolean determining whether to write logs to
	// the console. This field is set by methods on this type.
	logToConsoleFn func(configGetter) bool

	// logFormatJSONFn returns a boolean determining whether logs should be
	// written in JSON format.
	logFormatJSONFn func(configGetter) bool
}

// configGetter is a subset of the comp/core/config component, able to get
// config values for the xxxFn fields in LogParams.  comp/core/log uses
// this interface to get parameters that may depend on a configuration value.
type configGetter interface {
	GetString(key string) string
	GetBool(key string) bool
}

// ForOneShot sets up logging parameters for a one-shot app.
//
// If overrideFromEnv is set, then DD_LOG_LEVEL will override the given level.
//
// Otherwise, file logging is disabled, syslog is disabled, console logging is
// enabled, and JSON formatting is disabled.
func ForOneShot(loggerName, level string, overrideFromEnv bool) Params {
	params := Params{}
	params.loggerName = loggerName
	if overrideFromEnv {
		params.logLevelFn = func(configGetter) string {
			value, found := os.LookupEnv("DD_LOG_LEVEL")
			if !found {
				return level
			}
			return value
		}
	} else {
		params.logLevelFn = func(configGetter) string { return level }
	}
	params.logFileFn = func(configGetter) string { return "" }
	params.logSyslogURIFn = func(configGetter) string { return "" }
	params.logSyslogRFCFn = func(configGetter) bool { return false }
	params.logToConsoleFn = func(configGetter) bool { return true }
	params.logFormatJSONFn = func(configGetter) bool { return false }
	return params
}

// ForDaemon sets up logging parameters for a daemon app.
//
// The log level is set based on the `log_level` config parameter.
//
// The log file is set based on the logFileConfig config parameter,
// or disabled if `disable_file_logging` is set.
//
// On platforms which support it, syslog is enabled if `log_to_syslog` is set,
// using `syslog_uri` or defaulting to "unixgram:///dev/log" if that is empty.
// The `syslog_rfc` config parameter determines whether this produces 5424-compliant
// output.
//
// Console logging is enabled if `log_to_console` is set.  Lots are formatted
// as JSON if `log_format_json` is set.
func ForDaemon(loggerName, logFileConfig, defaultLogFile string) Params {
	params := Params{}
	params.loggerName = loggerName
	params.logLevelFn = func(g configGetter) string { return g.GetString("log_level") }
	params.logFileFn = func(g configGetter) string {
		if g.GetBool("disable_file_logging") {
			return ""
		}
		logFile := g.GetString(logFileConfig)
		if logFile == "" {
			logFile = defaultLogFile
		}
		return logFile
	}
	params.logSyslogURIFn = func(g configGetter) string {
		if runtime.GOOS == "windows" {
			return "" // syslog not supported on Windows
		}
		enabled := g.GetBool("log_to_syslog")
		uri := g.GetString("syslog_uri")

		if !enabled {
			return ""
		}

		if uri == "" {
			if runtime.GOOS == "darwin" {
				return "unixgram:///var/run/syslog"
			}
			return "unixgram:///dev/log"
		}

		return uri
	}
	params.logSyslogRFCFn = func(g configGetter) bool { return g.GetBool("syslog_rfc") }
	params.logToConsoleFn = func(g configGetter) bool { return g.GetBool("log_to_console") }
	params.logFormatJSONFn = func(g configGetter) bool { return g.GetBool("log_format_json") }
	return params
}

// LogToFile modifies the parameters to set the destination log file, overriding any
// previous logfile parameter.
func (params *Params) LogToFile(logFile string) {
	params.logFileFn = func(configGetter) string { return logFile }
}

// LogToConsole modifies the parameters to toggle logging to console
func (params *Params) LogToConsole(logToConsole bool) {
	params.logToConsoleFn = func(configGetter) bool { return logToConsole }
}

// LoggerName is the name that appears in the logfile
func (params Params) LoggerName() string {
	return params.loggerName
}

// These functions are used in unit tests.

// LogLevelFn returns the log level
func (params Params) LogLevelFn(c configGetter) string {
	return params.logLevelFn(c)
}

// LogFileFn returns the log file
func (params Params) LogFileFn(c configGetter) string {
	return params.logFileFn(c)
}

// IsLogLevelFnSet returns whether the logLevelFn field is set
func (params Params) IsLogLevelFnSet() bool {
	return params.logLevelFn != nil
}

// LogSyslogURIFn returns the syslog URI
func (params Params) LogSyslogURIFn(c configGetter) string {
	return params.logSyslogURIFn(c)
}

// LogSyslogRFCFn returns a boolean determining whether to use syslog RFC 5424
func (params Params) LogSyslogRFCFn(c configGetter) bool {
	return params.logSyslogRFCFn(c)
}

// LogToConsoleFn returns a boolean determining whether to write logs to the console
func (params Params) LogToConsoleFn(c configGetter) bool {
	return params.logToConsoleFn(c)
}

// LogFormatJSONFn returns a boolean determining whether logs should be written in JSON format
func (params Params) LogFormatJSONFn(c configGetter) bool {
	return params.logFormatJSONFn(c)
}
