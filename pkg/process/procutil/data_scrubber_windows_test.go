// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package procutil

import (
	"testing"
)

func TestStripArguments(t *testing.T) {
	testCases := []struct {
		cmdline []string
		expected    []string
	}{
		// Main cases samples
		{
			cmdline: []string{"python ~/test/run.py --password=1234 -password 1234 -open_password=admin -consul_token 2345 -blocked_from_yaml=1234 &"},
			expected: []string{"python"},
		},
		{
			cmdline: []string{"java -password      1234"},
			expected: []string{"java"},
		},
		{
			cmdline: []string{"agent password:1234"},
			expected: []string{"agent"},
		},
		{
			cmdline: []string{"agent", "-password", "1234"},
			expected: []string{"agent"},
		},
		{
			cmdline: []string{"C:\\Program Files\\Datadog\\agent.com"},
			expected: []string{"C:\\Program Files\\Datadog\\agent.com"},
		},
		{
			cmdline: []string{"C:\\Program Files\\Datadog\\agent.exe check process"},
			expected: []string{"C:\\Program Files\\Datadog\\agent.exe"},
		},
		{
			cmdline: []string{"C:\\Program Files\\Datadog\\agent.bat", "check", "process"},
			expected: []string{"C:\\Program Files\\Datadog\\agent.bat"},
		},
		{
			cmdline: []string{"\"C:\\Program Files\\Datadog\\agent.cmd\" check process"},
			expected: []string{"C:\\Program Files\\Datadog\\agent.cmd"},
		},
		// String matching extension structure
		{
			cmdline: []string{"C:\\Program File\\Datexedog\\agent.exe check process"},
			expected: []string{"C:\\Program File\\Datexedog\\agent.exe"},
		},
		// Mixed Variables
		{
			cmdline: []string{"C:\\Program Files\\agent.vbs check process"},
			expected: []string{"C:\\Program Files\\agent.vbs"},
		},
		{
			cmdline: []string{"C:\\Program Files\\Datadog\\agent.js", "check", "process"},
			expected: []string{"C:\\Program Files\\Datadog\\agent.js"},
		},
		{
			cmdline: []string{"C:\\Program Files\\Datadog\\agent.jse check process"},
			expected: []string{"C:\\Program Files\\Datadog\\agent.jse"},
		},
		{
			cmdline: []string{"\"C:\\Program Files\\Datadog\\agent.wsf\" check process"},
			expected: []string{"C:\\Program Files\\Datadog\\agent.wsf"},
		},
		{
			cmdline: []string{"C:\\Program Files\\Datadog\\agent.wsh check process"},
			expected: []string{"C:\\Program Files\\Datadog\\agent.wsh"},
		},
		{
			cmdline: []string{"\"C:\\Program Files\\Datadog\\agent.psc1\" check process"},
			expected: []string{"C:\\Program Files\\Datadog\\agent.psc1"},
		},
		{
			cmdline: []string{"\"C:\\Program Files\\agent\" check process"},
			expected: []string{"C:\\Program Files\\agent"},
		},
	}

	scrubber := setupDataScrubber(t)
	scrubber.StripAllArguments = true

	for _, tc := range testCases {
		cmdline := scrubber.stripArguments(tc.cmdline)
		if got := cmdline; got[0] != tc.expected[0] {
			t.Errorf("got %s; expected %s", got, tc.expected)
		}
	}
}

func TestFindEmbeddedQuotes(t *testing.T) {
	testCases := []struct {
		cmdline string
		expected    string
	}{
		{
			cmdline: "\"C:\\Program Files\\Datadog\\agent.cmd\" check process ",
			expected:    "C:\\Program Files\\Datadog\\agent.cmd",
		},
	}

	for _, tc := range testCases {
		cmdline := findEmbeddedQuotes(tc.cmdline)
		if got := cmdline; got[0] != tc.expected[0] {
			t.Errorf("got %s; expected %s", got, tc.expected)
		}
	}
}

func TestExtensionParser(t *testing.T) {
	testCases := []struct {
		cmdline string
		expected    string
	}{
		{
			cmdline: "python ~/test/run.py --password=1234 -password 1234 -open_password=admin -consul_token 2345 -blocked_from_yaml=1234 & ",
			expected:    "python",
		},
	}

	for _, tc := range testCases {
		cmdline := extensionParser(tc.cmdline, winDotExec)
		if got := cmdline; got[0] != tc.expected[0] {
			t.Errorf("got %s; expected %s", got, tc.expected)
		}
	}
}
