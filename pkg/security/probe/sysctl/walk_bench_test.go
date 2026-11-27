// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package sysctl

import "testing"

func BenchmarkWalkProcSys(b *testing.B) {
	var n int
	b.ReportAllocs()
	for b.Loop() {
		n = 0
		if err := walkSystemControls("/proc/sys", nil, func(_, _ []byte) { n++ }); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(n), "params/op")
}
