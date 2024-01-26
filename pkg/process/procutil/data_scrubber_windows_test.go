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

func TestStripArguments(b *testing.B) {

	cases := []struct {
		cmdline      []string
		striplessCmdline []string
	}{	
// windows case 1
	{[]string{"C:\\Program Files\\Datadog\\agent.exe"}, []string{"C:\\Program Files\\Datadog\\agent.exe"}},

// windows case 2
	{[]string{"C:\\Program Files\\Datadog\\agent.exe check, process"}, []string{"C:\\Program Files\\Datadog\\agent.exe"}},

// windows case 3 
		{[]string{"C:\\Program Files\\Datadog\\agent.exe", "check", "process"}, []string{"C:\\Program Files\\Datadog\\agent.exe"}},

// windows case 4 
	{[]string{"\\\"C:\\Program Files\\Datadog\\agent.exe\\\" check process"}, []string{"C:\\Program Files\\Datadog\\agent.exe"}},	
	}

	for i := range cases {
		fp := &Process{Cmdline: cases[i].cmdline}
		cases[i].cmdline.stripArguments
		assert.Equal(t, cases[i].triplessCmdline, cases[i].cmdline)
	}
}	
