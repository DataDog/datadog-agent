// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package procutil

import (
	"testing"
)

func TestStripArguments(t *testing.T) {

	testCases := []struct {
		cmdline []string
		want    []string
	}{
		{
			cmdline: []string{"agent", "-password", "1234"},
			want:    []string{"agent"},
		},
		{
			cmdline: []string{"fitz", "-consul_token", "1234567890"},
			want:    []string{"fitz"},
		},
		{
			cmdline: []string{"fitz", "--consul_token", "1234567890"},
			want:    []string{"fitz"},
		},
		{
			cmdline: []string{"python ~/test/run.py -open_password=admin -consul_token 2345 -blocked_from_yamt=1234 &"},
			want:    []string{"python"},
		},
		{
			cmdline: []string{"java -password      1234"},
			want:    []string{"java"},
		},
		{
			cmdline: []string{"agent password:1234"},
			want:    []string{"agent"},
		},
	}

	scrubber := setupDataScrubber(t)
	scrubber.StripAllArguments = true

	for _, tc := range testCases {
		cmdline := scrubber.stripArguments(tc.cmdline)
		if got := cmdline; got[0] != tc.want[0] {
			t.Errorf("got %s; want %s", got, tc.want)
		}
	}
}
