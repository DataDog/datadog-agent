// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build darwin

package portlist

import (
	"testing"
)

func BenchmarkGetList(b *testing.B) {
	benchmarkGetList(b, false)
}

func BenchmarkGetListIncremental(b *testing.B) {
	benchmarkGetList(b, true)
}

func benchmarkGetList(b *testing.B, incremental bool) {
	b.ReportAllocs()
	var p Poller
	p.init()
	if p.initErr != nil {
		b.Skip(p.initErr)
	}
	b.Cleanup(func() { p.Close() })
	for i := 0; i < b.N; i++ {
		pl, err := p.getList()
		if err != nil {
			b.Fatal(err)
		}
		if incremental {
			p.prev = pl
		}
	}
}
