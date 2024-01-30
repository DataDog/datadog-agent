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

	cases := []struct {
		cmdline       []string
		noArgsCmdline []string
	}{
		{[]string{"C:\\Program Files\\Datadog\\agent.com"}, []string{"C:\\Program Files\\Datadog\\agent.com"}},
		{[]string{"C:\\Program Files\\Datadog\\agent.exe check, process"}, []string{"C:\\Program Files\\Datadog\\agent.exe"}},
		{[]string{"C:\\Program Files\\Datadog\\agent.bat", "check", "process"}, []string{"C:\\Program Files\\Datadog\\agent.bat"}},
		{[]string{"\"C:\\Program Files\\Datadog\\agent.cmd\\\" check process"}, []string{"C:\\Program Files\\Datadog\\agent.cmd"}},
		{[]string{"\"C:\\Program Files\\Datadog\\agent.vbs\\\" check process"}, []string{"C:\\Program Files\\Datadog\\agent.vbs"}},
		{[]string{"\"C:\\Program Files\\Datadog\\agent.vbe\\\" check process"}, []string{"C:\\Program Files\\Datadog\\agent.vbe"}},
		{[]string{"\"C:\\Program Files\\Datadog\\agent.js\\\" check process"}, []string{"C:\\Program Files\\Datadog\\agent.js"}},
		{[]string{"\"C:\\Program Files\\Datadog\\agent.jse\\\" check process"}, []string{"C:\\Program Files\\Datadog\\agent.jse"}},
		{[]string{"\"C:\\Program Files\\Datadog\\agent.wsf\\\" check process"}, []string{"C:\\Program Files\\Datadog\\agent.wsf"}},
		{[]string{"\"C:\\Program Files\\Datadog\\agent.wsh\\\" check process"}, []string{"C:\\Program Files\\Datadog\\agent.wsh"}},
		{[]string{"\"C:\\Program Files\\Datadog\\agent.psc1\\\" check process"}, []string{"C:\\Program Files\\Datadog\\agent.psc1"}},
	}
	scrubber := setupDataScrubber(t)
	scrubber.StripAllArguments = true

	for i := range cases {
		cmdline := cases[i].cmdline
		cases[i].cmdline = scrubber.stripArguments(cmdline)
		assert.Equal(t, cases[i].noArgsCmdline, cases[i].cmdline)
	}
}
