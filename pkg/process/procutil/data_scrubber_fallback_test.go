// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows
package procutil

import (

	"testing"
	"github.com/stretchr/testify/assert"
)

func TestStripArguments(t *testing.T) {

	cases := []struct {
		cmdline      []string
		noArgsCmdline []string
	}{	
		{[]string{"agent", "-password", "1234"}, []string{"agent"}},
		{[]string{"fitz", "-consul_token", "1234567890"}, []string{"fitz"}},
		{[]string{"fitz", "--consul_token", "1234567890"}, []string{"fitz"}},
		{[]string{"python ~/test/run.py -open_password=admin -consul_token 2345 -blocked_from_yamt=1234 &"},[]string{"python"},},
		{[]string{"java -password      1234"}, []string{"java"}},
		{[]string{"agent password:1234"}, []string{"agent"}},
	}

	scrubber := setupDataScrubber(t)
	scrubber.StripAllArguments = true

	for i := range cases {
		cmdline := cases[i].cmdline
		cases[i].cmdline = scrubber.stripArguments(cmdline)
		assert.Equal(t, cases[i].noArgsCmdline, cases[i].cmdline)
	}
}
