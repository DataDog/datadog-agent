// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package portlist

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/google/go-cmp/cmp"
)

func TestPortEqualLessThan(t *testing.T) {
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
		assert.Equal(t, got, tt.want)

		lessBack := tt.b.lessThan(&tt.a)
		if got && lessBack {
			t.Errorf("%s: both a and b report being less than each other", tt.name)
		}

		wantEqual := !got && !lessBack
		gotEqual := tt.a.equal(&tt.b)
		assert.Equal(t, gotEqual, wantEqual)
	}
}

func Test_sortAndDedup(t *testing.T) {
	pl := List{
		{Proto: "tcp", Port: 100, Process: "proc1"},
		{Proto: "udp", Port: 100, Process: "proc2"},
		{Proto: "udp", Port: 100, Process: "proc2"},
		{Proto: "tcp", Port: 101, Process: "proc3"},
	}

	want := List{
		{Proto: "tcp", Port: 100, Process: "proc1"},
		{Proto: "udp", Port: 100, Process: "proc2"},
		{Proto: "tcp", Port: 101, Process: "proc3"},
	}
	got := sortAndDedup(pl)

	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("sortAndDedup mismatch (-want +got):\n%s", diff)
	}
}
