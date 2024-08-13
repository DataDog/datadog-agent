// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package procutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripArguments(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cmdline  []string
		expected string
	}{
		{
			name:     "Empty string",
			cmdline:  []string{""},
			expected: "",
		},
		{
			name:     "OS parse",
			cmdline:  []string{"agent", "-password", "1234"},
			expected: "agent",
		},
		{
			name:     "Windows exec + OS parse",
			cmdline:  []string{"C:\\Program Files\\Datadog\\agent.bat", "check", "process"},
			expected: "C:\\Program Files\\Datadog\\agent.bat",
		},
		{
			name:     "No OS parse",
			cmdline:  []string{"python ~/test/run.py --password=1234 -password 1234 -open_password=admin -consul_token 2345 -blocked_from_yaml=1234 &"},
			expected: "python",
		},
		{
			name:     "No OS parse + whitespace",
			cmdline:  []string{"java -password      1234"},
			expected: "java",
		},
		{
			name:     "No OS parse + Optional dash args",
			cmdline:  []string{"agent password:1234"},
			expected: "agent",
		},
		{
			name:     "Windows exec",
			cmdline:  []string{"C:\\Program Files\\Datadog\\agent.com"},
			expected: "C:\\Program Files\\Datadog\\agent.com",
		},
		{
			name:     "Windows exec + args",
			cmdline:  []string{"C:\\Program Files\\Datadog\\agent.exe check process"},
			expected: "C:\\Program Files\\Datadog\\agent.exe",
		},
		{
			name:     "Windows exec + paired quotes",
			cmdline:  []string{"\"C:\\Program Files\\Datadog\\agent.cmd\" check process"},
			expected: "C:\\Program Files\\Datadog\\agent.cmd",
		},
		{
			name:     "Paired quotes",
			cmdline:  []string{"\"C:\\Program Files\\agent\" check process"},
			expected: "C:\\Program Files\\agent",
		},
	} {

		scrubber := setupDataScrubber(t)
		scrubber.StripAllArguments = true

		t.Run(tc.name, func(t *testing.T) {
			cmdline := scrubber.stripArguments(tc.cmdline)
			assert.Equal(t, cmdline[0], tc.expected)
		})
	}
}

func TestFindEmbeddedQuotes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cmdline  string
		expected string
	}{
		{
			name:     "Paired quotes",
			cmdline:  "\"C:\\Program Files\\Datadog\\agent.cmd\" check process ",
			expected: "C:\\Program Files\\Datadog\\agent.cmd",
		},
		{
			name:     "One quote",
			cmdline:  "\"C:\\Program Files\\Datadog\\agent.cmd check process ",
			expected: "\"C:\\Program Files\\Datadog\\agent.cmd check process ",
		},
		{
			name:     "Empty string",
			cmdline:  "",
			expected: "",
		},
	} {

		t.Run(tc.name, func(t *testing.T) {
			actual := findEmbeddedQuotes(tc.cmdline)
			assert.Equal(t, actual, tc.expected)
		})
	}
}

func TestExtensionParser(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cmdline  string
		expected string
	}{
		{
			name:     "Extension not found",
			cmdline:  "python ~/test/run.py --password=1234 -password 1234 -open_password=admin -consul_token 2345 -blocked_from_yaml=1234 & ",
			expected: "",
		},
		{
			name:     "Extension at end of line",
			cmdline:  "C:\\Program Files\\Datadog\\agent.com",
			expected: "C:\\Program Files\\Datadog\\agent.com",
		},
		{
			name:     "Extension in first token",
			cmdline:  "C:\\Program Files\\Datadog\\agent.cmd check process",
			expected: "C:\\Program Files\\Datadog\\agent.cmd",
		},
		{
			name:     "Multiple extensions",
			cmdline:  "C:\\Program Files\\Datadog\\agent.exec.process.cmd check process",
			expected: "C:\\Program Files\\Datadog\\agent.exec.process.cmd",
		},
		{
			name:     "Misformed extension",
			cmdline:  "C:\\Program File\\Datexedog\\agent.exe check process",
			expected: "C:\\Program File\\Datexedog\\agent.exe",
		},
		{
			name:     "vbs extension",
			cmdline:  "C:\\Program Files\\agent.vbs check process",
			expected: "C:\\Program Files\\agent.vbs",
		},
		{
			name:     "jse extension",
			cmdline:  "C:\\Program Files\\Datadog\\agent.jse check process",
			expected: "C:\\Program Files\\Datadog\\agent.jse",
		},
		{
			name:     "wsf extension",
			cmdline:  "C:\\Program Files\\Datadog\\agent.wsf check process",
			expected: "C:\\Program Files\\Datadog\\agent.wsf",
		},
		{
			name:     "wsh extension",
			cmdline:  "C:\\Program Files\\Datadog\\agent.wsh check process",
			expected: "C:\\Program Files\\Datadog\\agent.wsh",
		},
		{
			name:     "psc1 extension",
			cmdline:  "C:\\Program Files\\Datadog\\agent.psc1 check process",
			expected: "C:\\Program Files\\Datadog\\agent.psc1",
		},
		{
			name:     "bat extension",
			cmdline:  "C:\\Program Files\\Datadog\\agent.bat check process",
			expected: "C:\\Program Files\\Datadog\\agent.bat",
		},
		{
			name:     "js extension",
			cmdline:  "C:\\Program Files\\Datadog\\agent.js check process",
			expected: "C:\\Program Files\\Datadog\\agent.js",
		},
		{
			name:     "com extension",
			cmdline:  "C:\\Program Files\\Datadog\\agent.com check process",
			expected: "C:\\Program Files\\Datadog\\agent.com",
		},
	} {

		t.Run(tc.name, func(t *testing.T) {
			actual := extensionParser(tc.cmdline, executibleExtensions)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
