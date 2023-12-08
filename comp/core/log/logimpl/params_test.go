// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logimpl

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

type getter struct {
	strs  map[string]string
	bools map[string]bool
}

func (g *getter) GetString(k string) string {
	return g.strs[k]
}

func (g *getter) GetBool(k string) bool {
	return g.bools[k]
}

func TestForOneShot_noOverride(t *testing.T) {
	params := ForOneShot("TEST", "trace", false)
	g := &getter{}
	t.Setenv("DD_LOG_LEVEL", "debug")

	require.Equal(t, "TEST", params.loggerName)
	require.Equal(t, "trace", params.logLevelFn(g))
	require.Equal(t, "", params.logFileFn(g))
	require.Equal(t, "", params.logSyslogURIFn(g))
	require.Equal(t, false, params.logSyslogRFCFn(g))
	require.Equal(t, true, params.logToConsoleFn(g))
	require.Equal(t, false, params.logFormatJSONFn(g))
}

func TestForOneShot_override(t *testing.T) {
	params := ForOneShot("TEST", "trace", true)
	g := &getter{}
	t.Setenv("DD_LOG_LEVEL", "debug")

	require.Equal(t, "TEST", params.loggerName)
	require.Equal(t, "debug", params.logLevelFn(g))
	require.Equal(t, "", params.logFileFn(g))
	require.Equal(t, "", params.logSyslogURIFn(g))
	require.Equal(t, false, params.logSyslogRFCFn(g))
	require.Equal(t, true, params.logToConsoleFn(g))
	require.Equal(t, false, params.logFormatJSONFn(g))
}

func TestForDaemon_windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip()
	}
	params := ForDaemon("TEST", "unused", "unused")
	g := &getter{
		strs: map[string]string{
			"log_level": "trace",
		},
		bools: map[string]bool{
			"log_to_syslog": true, // enabled, but doesn't exist on windows
		},
	}

	require.Equal(t, "TEST", params.loggerName)
	require.Equal(t, "trace", params.logLevelFn(g))
	require.Equal(t, "unused", params.logFileFn(g)) // default log file, since log_file isn't set in g
	require.Equal(t, "", params.logSyslogURIFn(g))  // always empty on Windows
	require.Equal(t, false, params.logSyslogRFCFn(g))
	require.Equal(t, false, params.logToConsoleFn(g))  // not set in g
	require.Equal(t, false, params.logFormatJSONFn(g)) // not set in g
}

func TestForDaemon_linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip()
	}
	makeGetter := func() *getter {
		return &getter{
			strs: map[string]string{
				"log_level":  "trace",
				"log_file":   "",
				"syslog_uri": "",
			},
			bools: map[string]bool{
				"disable_file_logging": false,
				"log_to_syslog":        false,
				"syslog_rfc":           true,
				"log_to_console":       false,
				"log_format_json":      true,
			},
		}
	}

	t.Run("log_file config", func(t *testing.T) {
		params := ForDaemon("TEST", "log_file", "unused")
		g := makeGetter()
		g.strs["log_file"] = "/foo/bar"
		require.Equal(t, "TEST", params.loggerName)
		require.Equal(t, "trace", params.logLevelFn(g))
		require.Equal(t, "/foo/bar", params.logFileFn(g))
		require.Equal(t, "", params.logSyslogURIFn(g))
		require.Equal(t, true, params.logSyslogRFCFn(g))
		require.Equal(t, false, params.logToConsoleFn(g))
		require.Equal(t, true, params.logFormatJSONFn(g))
	})

	t.Run("log_file default", func(t *testing.T) {
		params := ForDaemon("TEST", "log_file", "/default/log")
		g := makeGetter()
		g.strs["log_file"] = ""
		require.Equal(t, "TEST", params.loggerName)
		require.Equal(t, "trace", params.logLevelFn(g))
		require.Equal(t, "/default/log", params.logFileFn(g))
		require.Equal(t, "", params.logSyslogURIFn(g))
		require.Equal(t, true, params.logSyslogRFCFn(g))
		require.Equal(t, false, params.logToConsoleFn(g))
		require.Equal(t, true, params.logFormatJSONFn(g))
	})

	t.Run("disable_file_logging", func(t *testing.T) {
		params := ForDaemon("TEST", "log_file", "/default/log")
		g := makeGetter()
		g.bools["disable_file_logging"] = true
		require.Equal(t, "TEST", params.loggerName)
		require.Equal(t, "trace", params.logLevelFn(g))
		require.Equal(t, "", params.logFileFn(g))
		require.Equal(t, "", params.logSyslogURIFn(g))
		require.Equal(t, true, params.logSyslogRFCFn(g))
		require.Equal(t, false, params.logToConsoleFn(g))
		require.Equal(t, true, params.logFormatJSONFn(g))
	})

	t.Run("log to syslog", func(t *testing.T) {
		params := ForDaemon("TEST", "log_file", "/default/log")
		g := makeGetter()
		g.bools["log_to_syslog"] = true
		require.Equal(t, "TEST", params.loggerName)
		require.Equal(t, "trace", params.logLevelFn(g))
		require.Equal(t, "/default/log", params.logFileFn(g))
		require.Equal(t, "unixgram:///dev/log", params.logSyslogURIFn(g))
		require.Equal(t, true, params.logSyslogRFCFn(g))
		require.Equal(t, false, params.logToConsoleFn(g))
		require.Equal(t, true, params.logFormatJSONFn(g))
	})

	t.Run("log to syslog with uri", func(t *testing.T) {
		params := ForDaemon("TEST", "log_file", "/default/log")
		g := makeGetter()
		g.bools["log_to_syslog"] = true
		g.strs["syslog_uri"] = "test:///"
		require.Equal(t, "TEST", params.loggerName)
		require.Equal(t, "trace", params.logLevelFn(g))
		require.Equal(t, "/default/log", params.logFileFn(g))
		require.Equal(t, "test:///", params.logSyslogURIFn(g))
		require.Equal(t, true, params.logSyslogRFCFn(g))
		require.Equal(t, false, params.logToConsoleFn(g))
		require.Equal(t, true, params.logFormatJSONFn(g))
	})
}

func TestLogToFile(t *testing.T) {
	params := ForOneShot("TEST", "trace", true)
	params.LogToFile("/some/file")
	g := &getter{}

	require.Equal(t, "/some/file", params.logFileFn(g))
}
