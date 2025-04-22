// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package seelog provides the configuration for the logger
package seelog

import (
	"bytes"
	"flag"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// run `go test ./setup/internal/... -generate` to regenerate the expected outputs in testdata/
var generate = flag.Bool("generate", false, "generates output to testdata/ if set")

func TestSeelogConfig(t *testing.T) {
	// if you change the test cases or the expected format, you'll need to regenerate the expected output
	testCases := []struct {
		testName string

		loggerName   string
		logLevel     string
		format       string
		jsonFormat   string
		commonFormat string
		syslogRFC    bool

		syslogURI string
		usetTLS   bool

		logfile  string
		maxsize  uint
		maxrolls uint

		consoleLoggingEnabled bool
	}{
		{
			testName:              "basic",
			loggerName:            "CORE",
			logLevel:              "info",
			format:                "common",
			commonFormat:          "%Date(2006-01-02T15:04:05.000) | CORE | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %ExtraTextContext%Msg%n",
			consoleLoggingEnabled: true,
		},
		{
			testName:     "configured",
			loggerName:   "JMXFetch",
			logLevel:     "debug",
			format:       "json",
			commonFormat: "%Msg%n",
			jsonFormat:   `{"msg":%QuoteMsg}%n`,
		},
		{
			testName:              "syslog",
			loggerName:            "CORE",
			logLevel:              "info",
			format:                "syslog-common",
			commonFormat:          "%CustomSyslogHeader(20,false) CORE | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %ExtraTextContext%Msg%n",
			syslogRFC:             false,
			syslogURI:             "udp://localhost:514",
			usetTLS:               false,
			consoleLoggingEnabled: true,
		},
		{
			testName:              "file",
			loggerName:            "CORE",
			logLevel:              "info",
			format:                "common",
			commonFormat:          "%Date(2006-01-02T15:04:05.000) | CORE | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %ExtraTextContext%Msg%n",
			jsonFormat:            `{"agent":"CORE","level":"%LEVEL","relfile":"%ShortFilePath","line":"%Line","msg":"%Msg"%ExtraJSONContext}%n`,
			logfile:               "/var/log/datadog/agent.log",
			maxsize:               100,
			maxrolls:              10,
			consoleLoggingEnabled: true,
		},
		{
			testName:              "all",
			loggerName:            "CORE",
			logLevel:              "info",
			format:                "common",
			commonFormat:          "%Date(2006-01-02T15:04:05.000) | CORE | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %ExtraTextContext%Msg%n",
			jsonFormat:            `{"agent":"CORE","time":"%Date(2006-01-02 15:04:05 MST)","level":"%LEVEL","file":"%ShortFilePath","line":"%Line","func":"%FuncShort","msg":%QuoteMsg%ExtraJSONContext}%n`,
			syslogRFC:             true,
			syslogURI:             "udp://localhost:514",
			usetTLS:               true,
			logfile:               "/var/log/datadog/agent.log",
			maxsize:               100,
			maxrolls:              10,
			consoleLoggingEnabled: true,
		},
		{
			testName:              "off",
			loggerName:            "CORE",
			logLevel:              "off",
			format:                "common",
			commonFormat:          "%Date(2006-01-02T15:04:05.000) | CORE | %LEVEL | (%ShortFilePath:%Line in %FuncShort) | %ExtraTextContext%Msg%n",
			consoleLoggingEnabled: false,
		},
		{
			testName:              "special_characters",
			loggerName:            `"'<a&b>'"`,
			logLevel:              `"'<a&b>'"`,
			format:                `"'<a&b>'"`,
			commonFormat:          `"'<a&b>'"`,
			jsonFormat:            `"'<a&b>'"`,
			syslogURI:             `"'<a&b>'"`,
			logfile:               `"'<a&b>'"`,
			consoleLoggingEnabled: true,
			syslogRFC:             true,
			usetTLS:               true,
			maxsize:               100,
			maxrolls:              10,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			config := NewSeelogConfig(tc.loggerName, tc.logLevel, tc.format, tc.jsonFormat, tc.commonFormat, tc.syslogRFC)
			config.ConfigureSyslog(tc.syslogURI, tc.usetTLS)
			config.EnableFileLogging(tc.logfile, tc.maxsize, tc.maxrolls)
			config.EnableConsoleLog(tc.consoleLoggingEnabled)

			testSeelogConfig(t, config, tc.testName)
		})
	}
}

func testSeelogConfig(t *testing.T, config *Config, testName string) {
	cfg, err := config.Render()
	require.NoError(t, err)

	expectedFileName := "testdata/" + testName + ".xml"

	// if the flag is set, update the fixtures containing the expected output
	// otherwise just compare the generated output with the expected one
	if *generate {
		err := os.WriteFile(expectedFileName, []byte(cfg), 0644)
		require.NoError(t, err)
		return
	}

	expected, err := os.ReadFile(expectedFileName)
	require.NoError(t, err)

	if runtime.GOOS == "windows" {
		expected = bytes.ReplaceAll(expected, []byte("\r\n"), []byte("\n"))
	}

	require.Equal(t, string(expected), cfg)
}
