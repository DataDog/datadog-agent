// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package internal

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

func TestLogForOneShot_noOverride(t *testing.T) {
	params := BundleParams{}.LogForOneShot("TEST", "trace", false)
	g := &getter{}
	t.Setenv("DD_LOG_LEVEL", "debug")

	require.Equal(t, "TEST", params.LoggerName)
	require.Equal(t, "trace", params.LogLevelFn(g))
	require.Equal(t, "", params.LogFileFn(g))
	require.Equal(t, "", params.LogSyslogURIFn(g))
	require.Equal(t, false, params.LogSyslogRFCFn(g))
	require.Equal(t, true, params.LogToConsoleFn(g))
	require.Equal(t, false, params.LogFormatJSONFn(g))
}

func TestLogForOneShot_override(t *testing.T) {
	params := BundleParams{}.LogForOneShot("TEST", "trace", true)
	g := &getter{}
	t.Setenv("DD_LOG_LEVEL", "debug")

	require.Equal(t, "TEST", params.LoggerName)
	require.Equal(t, "debug", params.LogLevelFn(g))
	require.Equal(t, "", params.LogFileFn(g))
	require.Equal(t, "", params.LogSyslogURIFn(g))
	require.Equal(t, false, params.LogSyslogRFCFn(g))
	require.Equal(t, true, params.LogToConsoleFn(g))
	require.Equal(t, false, params.LogFormatJSONFn(g))
}

func TestLogForDaemon_windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip()
	}
	params := BundleParams{}.LogForDaemon("TEST", "unused", "unused")
	g := &getter{
		strs: map[string]string{
			"log_level": "trace",
		},
		bools: map[string]bool{
			"log_to_syslog": true, // enabled, but doesn't exist on windows
		},
	}

	require.Equal(t, "TEST", params.LoggerName)
	require.Equal(t, "trace", params.LogLevelFn(g))
	require.Equal(t, "", params.LogFileFn(g))
	require.Equal(t, "", params.LogSyslogURIFn(g)) // still empty
	require.Equal(t, false, params.LogSyslogRFCFn(g))
	require.Equal(t, true, params.LogToConsoleFn(g))
	require.Equal(t, false, params.LogFormatJSONFn(g))
}

func TestLogForDaemon_linux(t *testing.T) {
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
		params := BundleParams{}.LogForDaemon("TEST", "log_file", "unused")
		g := makeGetter()
		g.strs["log_file"] = "/foo/bar"
		require.Equal(t, "TEST", params.LoggerName)
		require.Equal(t, "trace", params.LogLevelFn(g))
		require.Equal(t, "/foo/bar", params.LogFileFn(g))
		require.Equal(t, "", params.LogSyslogURIFn(g))
		require.Equal(t, true, params.LogSyslogRFCFn(g))
		require.Equal(t, false, params.LogToConsoleFn(g))
		require.Equal(t, true, params.LogFormatJSONFn(g))
	})

	t.Run("log_file default", func(t *testing.T) {
		params := BundleParams{}.LogForDaemon("TEST", "log_file", "/default/log")
		g := makeGetter()
		g.strs["log_file"] = ""
		require.Equal(t, "TEST", params.LoggerName)
		require.Equal(t, "trace", params.LogLevelFn(g))
		require.Equal(t, "/default/log", params.LogFileFn(g))
		require.Equal(t, "", params.LogSyslogURIFn(g))
		require.Equal(t, true, params.LogSyslogRFCFn(g))
		require.Equal(t, false, params.LogToConsoleFn(g))
		require.Equal(t, true, params.LogFormatJSONFn(g))
	})

	t.Run("disable_file_logging", func(t *testing.T) {
		params := BundleParams{}.LogForDaemon("TEST", "log_file", "/default/log")
		g := makeGetter()
		g.bools["disable_file_logging"] = true
		require.Equal(t, "TEST", params.LoggerName)
		require.Equal(t, "trace", params.LogLevelFn(g))
		require.Equal(t, "", params.LogFileFn(g))
		require.Equal(t, "", params.LogSyslogURIFn(g))
		require.Equal(t, true, params.LogSyslogRFCFn(g))
		require.Equal(t, false, params.LogToConsoleFn(g))
		require.Equal(t, true, params.LogFormatJSONFn(g))
	})

	t.Run("log to syslog", func(t *testing.T) {
		params := BundleParams{}.LogForDaemon("TEST", "log_file", "/default/log")
		g := makeGetter()
		g.bools["log_to_syslog"] = true
		require.Equal(t, "TEST", params.LoggerName)
		require.Equal(t, "trace", params.LogLevelFn(g))
		require.Equal(t, "/default/log", params.LogFileFn(g))
		require.Equal(t, "unixgram:///dev/log", params.LogSyslogURIFn(g))
		require.Equal(t, true, params.LogSyslogRFCFn(g))
		require.Equal(t, false, params.LogToConsoleFn(g))
		require.Equal(t, true, params.LogFormatJSONFn(g))
	})

	t.Run("log to syslog with uri", func(t *testing.T) {
		params := BundleParams{}.LogForDaemon("TEST", "log_file", "/default/log")
		g := makeGetter()
		g.bools["log_to_syslog"] = true
		g.strs["syslog_uri"] = "test:///"
		require.Equal(t, "TEST", params.LoggerName)
		require.Equal(t, "trace", params.LogLevelFn(g))
		require.Equal(t, "/default/log", params.LogFileFn(g))
		require.Equal(t, "test:///", params.LogSyslogURIFn(g))
		require.Equal(t, true, params.LogSyslogRFCFn(g))
		require.Equal(t, false, params.LogToConsoleFn(g))
		require.Equal(t, true, params.LogFormatJSONFn(g))
	})
}

func TestLogToFile(t *testing.T) {
	params := BundleParams{}.LogForOneShot("TEST", "trace", true).LogToFile("/some/file")
	g := &getter{}

	require.Equal(t, "/some/file", params.LogFileFn(g))
}
