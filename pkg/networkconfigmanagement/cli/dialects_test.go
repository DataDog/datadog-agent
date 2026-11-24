// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPrompts(t *testing.T) {
	for _, tc := range []struct {
		dialect *Dialect
		line    string
		want    bool
	}{
		{ASA, "fw01# ", true},
		{ASA, "fw01#", true},
		{ASA, "fw01>", true},
		{ASA, "cml-asav# ", true},
		{ASA, "fw01(config)# ", true},
		{ASA, "fw01/admin# ", true},
		{ASA, "fw01# show run", false},
		{ASA, "<--- More --->", false},
		{ASA, "", false},

		{IOS, "R1#", true},
		{IOS, "R1>", true},
		{IOS, "R1(config)#", true},
		{IOS, "R1(config-if)#", true},
		{IOS, "core-sw.lab#", true},
		{IOS, "R1#show run", false},
		{IOS, "Destination filename [startup-config]? ", false},
		{IOS, " --More-- ", false},

		{PanOS, "admin@PA-3420> ", true},
		{PanOS, "admin@PA-3420>", true},
		{PanOS, "admin@PA-3420# ", true},
		{PanOS, "ddncmpa@PA-440-MISSISAUGA-02(passive)> ", true},
		{PanOS, "admin@fw01.example.com(active)# ", true},
		{PanOS, "admin@PA-3420> show config running", false},
		{PanOS, "  <entry name=\"admin@x\">", false},
		{PanOS, "</config>", false},
		{PanOS, "R1#", false},
	} {
		assert.Equal(t, tc.want, tc.dialect.Prompt.MatchString(tc.line), "%s prompt %q", tc.dialect.Name, tc.line)
	}
}

func TestIOSErrors(t *testing.T) {
	for _, line := range []string{
		"% Invalid input detected at '^' marker.",
		"% Incomplete command.",
		`% Ambiguous command:  "sh i"`,
		"% Unknown command or computer name, or unable to find computer address",
		"% Bad IP address or host name",
		"%Error opening flash:foo (No such file or directory)",
		"% 10.0.0.0 overlaps with GigabitEthernet0/1",
		"% Command authorization failed.",
	} {
		assert.EqualError(t, IOS.CheckError("              ^\n"+line+"\n\n"), line)
	}
	for _, out := range []string{
		"% Warning: this is only a warning\n",
		"% Note: noted\n",
		"%SYS-5-CONFIG_I: Configured from console by admin\n",
		"Building configuration...\n[OK]\n",
	} {
		assert.NoError(t, IOS.CheckError(out), out)
	}
}

func TestASAErrors(t *testing.T) {
	assert.EqualError(t, ASA.CheckError("ERROR: % Invalid input detected at '^' marker.\n"),
		"ERROR: % Invalid input detected at '^' marker.")
	assert.NoError(t, ASA.CheckError("ASA Version 9.24(1)\nhostname FW01\n"))
	// A config line mentioning an error is not one.
	assert.NoError(t, ASA.CheckError("logging message 106023 level errors\n"))
}

func TestPanOSErrors(t *testing.T) {
	for _, line := range []string{
		"Invalid syntax.",
		"Unknown command: show confg",
		"Server error : configuration is locked",
	} {
		assert.EqualError(t, PanOS.CheckError("\n"+line+"\n\n"), line)
	}
	for _, out := range []string{
		"config {\n  mgt-config {\n  }\n}\n",
		"<config version=\"11.1.0\">\n</config>\n",
		// Indented config content is never an error line.
		"    description \"Invalid syntax.\";\n",
	} {
		assert.NoError(t, PanOS.CheckError(out), out)
	}
}
