// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

type getter struct {
	strs  map[string]string
	bools map[string]bool
	// configured tracks which keys were explicitly set by the "user", mirroring
	// pkgconfigmodel.Reader.IsConfigured: a key with a value that merely reflects
	// its registered default is not "configured".
	configured map[string]bool
}

func (g *getter) GetString(k string) string {
	return g.strs[k]
}

func (g *getter) GetBool(k string) bool {
	return g.bools[k]
}

func (g *getter) IsConfigured(k string) bool {
	return g.configured[k]
}

// setConfiguredString sets a string value as if the user had explicitly configured it.
func (g *getter) setConfiguredString(k, v string) {
	if g.strs == nil {
		g.strs = map[string]string{}
	}
	if g.configured == nil {
		g.configured = map[string]bool{}
	}
	g.strs[k] = v
	g.configured[k] = true
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
		g.setConfiguredString("log_file", "/foo/bar")
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

func TestForDaemon_logFile(t *testing.T) {
	makeGetter := func() *getter {
		return &getter{
			strs: map[string]string{
				"log_level": "trace",
			},
			bools: map[string]bool{
				"disable_file_logging": false,
			},
		}
	}

	t.Run("unconfigured log_file falls back to the daemon's default log file", func(t *testing.T) {
		// This mirrors how the config default for a daemon's log_file setting is
		// actually registered, e.g. BindEnvAndSetDefault("log_file", "${log_path}/agent.log"),
		// which resolves to defaultpaths.GetDefaultLogFile() rather than an empty string.
		// IsConfigured is false regardless of what that resolved default value looks like.
		params := ForDaemon("TEST", "log_file", "/default/daemon.log")
		g := makeGetter()
		g.strs["log_file"] = "/some/resolved/default.log"

		require.Equal(t, "/default/daemon.log", params.logFileFn(g))
	})

	t.Run("explicitly configured log_file is preserved", func(t *testing.T) {
		params := ForDaemon("TEST", "log_file", "/default/daemon.log")
		g := makeGetter()
		g.setConfiguredString("log_file", "/custom/path.log")

		require.Equal(t, "/custom/path.log", params.logFileFn(g))
	})

	t.Run("unconfigured log file config whose own default is empty falls back to the daemon's default log file", func(t *testing.T) {
		// This mirrors trace-agent's "apm_config.log_file", which is registered with
		// BindEnvAndSetDefault("apm_config.log_file", "", "DD_APM_LOG_FILE") -- i.e. its
		// own default is "", not defaultpaths.GetDefaultLogFile(). Some daemons (e.g. the
		// installer's "installer.log_file") aren't registered at all and also resolve to "".
		// Unlike a plain "logFile == ''" check, IsConfigured correctly falls back here too.
		params := ForDaemon("TRACE", "apm_config.log_file", "/default/trace-agent.log")
		g := makeGetter()
		g.strs["apm_config.log_file"] = ""

		require.Equal(t, "/default/trace-agent.log", params.logFileFn(g))
	})

	t.Run("explicitly configured empty-default log file config is preserved", func(t *testing.T) {
		params := ForDaemon("TRACE", "apm_config.log_file", "/default/trace-agent.log")
		g := makeGetter()
		g.setConfiguredString("apm_config.log_file", "/custom/path.log")

		require.Equal(t, "/custom/path.log", params.logFileFn(g))
	})

	t.Run("explicitly configured empty log_file is preserved, unlike a naive empty-string check", func(t *testing.T) {
		// If the user explicitly sets log_file to "" (e.g. to disable file logging via
		// config rather than disable_file_logging), IsConfigured still reports true, so
		// their choice is respected instead of silently falling back to the default.
		params := ForDaemon("TRACE", "apm_config.log_file", "/default/trace-agent.log")
		g := makeGetter()
		g.setConfiguredString("apm_config.log_file", "")

		require.Equal(t, "", params.logFileFn(g))
	})
}

func TestLogToFile(t *testing.T) {
	params := ForOneShot("TEST", "trace", true)
	params.LogToFile("/some/file")
	g := &getter{}

	require.Equal(t, "/some/file", params.logFileFn(g))
}
