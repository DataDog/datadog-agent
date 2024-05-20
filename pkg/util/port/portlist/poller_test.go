// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package portlist

import (
	"reflect"
	"testing"
)

func TestEqualLessThan(t *testing.T) {
	tests := []struct {
		name string
		a, b Port
		want bool
	}{
		{
			"Port a < b",
			Port{Proto: "tcp", Port: 100, Process: "proc1"},
			Port{Proto: "tcp", Port: 101, Process: "proc1"},
			true,
		},
		{
			"Port a > b",
			Port{Proto: "tcp", Port: 101, Process: "proc1"},
			Port{Proto: "tcp", Port: 100, Process: "proc1"},
			false,
		},
		{
			"Proto a < b",
			Port{Proto: "tcp", Port: 100, Process: "proc1"},
			Port{Proto: "udp", Port: 100, Process: "proc1"},
			true,
		},
		{
			"Proto a < b",
			Port{Proto: "udp", Port: 100, Process: "proc1"},
			Port{Proto: "tcp", Port: 100, Process: "proc1"},
			false,
		},
		{
			"Process a < b",
			Port{Proto: "tcp", Port: 100, Process: "proc1"},
			Port{Proto: "tcp", Port: 100, Process: "proc2"},
			true,
		},
		{
			"Process a > b",
			Port{Proto: "tcp", Port: 100, Process: "proc2"},
			Port{Proto: "tcp", Port: 100, Process: "proc1"},
			false,
		},
		{
			"Port evaluated first",
			Port{Proto: "udp", Port: 100, Process: "proc2"},
			Port{Proto: "tcp", Port: 101, Process: "proc1"},
			true,
		},
		{
			"Proto evaluated second",
			Port{Proto: "tcp", Port: 100, Process: "proc2"},
			Port{Proto: "udp", Port: 100, Process: "proc1"},
			true,
		},
		{
			"Process evaluated fourth",
			Port{Proto: "tcp", Port: 100, Process: "proc1"},
			Port{Proto: "tcp", Port: 100, Process: "proc2"},
			true,
		},
		{
			"equal",
			Port{Proto: "tcp", Port: 100, Process: "proc1"},
			Port{Proto: "tcp", Port: 100, Process: "proc1"},
			false,
		},
	}

	for _, tt := range tests {
		got := tt.a.lessThan(&tt.b)
		if got != tt.want {
			t.Errorf("%s: Equal = %v; want %v", tt.name, got, tt.want)
		}
		lessBack := tt.b.lessThan(&tt.a)
		if got && lessBack {
			t.Errorf("%s: both a and b report being less than each other", tt.name)
		}
		wantEqual := !got && !lessBack
		gotEqual := tt.a.equal(&tt.b)
		if gotEqual != wantEqual {
			t.Errorf("%s: equal = %v; want %v", tt.name, gotEqual, wantEqual)
		}
	}
}

func TestSortAndDedup(t *testing.T) {
	tests := []struct {
		name     string
		input    List
		expected List
	}{
		{
			"Simple Case",
			List{
				{Port: 80, Proto: "tcp", Process: "nginx"},
				{Port: 443, Proto: "tcp", Process: "nginx"},
				{Port: 80, Proto: "tcp", Process: "apache"},
				{Port: 80, Proto: "udp", Process: "apache"},
				{Port: 443, Proto: "tcp", Process: "nginx"},
			},
			List{
				{Port: 80, Proto: "tcp", Process: "apache"},
				{Port: 80, Proto: "udp", Process: "apache"},
				{Port: 443, Proto: "tcp", Process: "nginx"},
			},
		},
		{
			"Already Sorted",
			List{
				{Port: 22, Proto: "tcp", Process: "ssh"},
				{Port: 80, Proto: "tcp", Process: "nginx"},
				{Port: 443, Proto: "tcp", Process: "nginx"},
			},
			List{
				{Port: 22, Proto: "tcp", Process: "ssh"},
				{Port: 80, Proto: "tcp", Process: "nginx"},
				{Port: 443, Proto: "tcp", Process: "nginx"},
			},
		},
		{
			"No Duplicates",
			List{
				{Port: 80, Proto: "tcp", Process: "nginx"},
				{Port: 8080, Proto: "tcp", Process: "node"},
			},
			List{
				{Port: 80, Proto: "tcp", Process: "nginx"},
				{Port: 8080, Proto: "tcp", Process: "node"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sortAndDedup(tt.input)
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("sortAndDedup(%v) = %v; want %v", tt.input, result, tt.expected)
			}
		})
	}
}
