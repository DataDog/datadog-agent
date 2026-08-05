// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package numamonitoring

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestParseNUMAMaps(t *testing.T) {
	input := strings.NewReader(`00400000 default file=/bin/a mapped=2 N0=1 N1=1 kernelpagesize_kB=4
7f000000 interleave anon=3 dirty=3 N1=3 kernelpagesize_kB=2048
`)
	resident, err := parseNUMAMaps(input, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := resident[0], uint64(4096); got != want {
		t.Fatalf("node 0 bytes = %d, want %d", got, want)
	}
	if got, want := resident[1], uint64(4096+3*2*1024*1024); got != want {
		t.Fatalf("node 1 bytes = %d, want %d", got, want)
	}
}

func TestDistributionsAndScores(t *testing.T) {
	runtime := distribution(map[int]uint64{0: 75, 1: 25})
	residency := distribution(map[int]uint64{0: 25, 1: 75})
	mismatch, ok := placementMismatch(runtime, residency)
	if !ok || math.Abs(mismatch-0.5) > 1e-9 {
		t.Fatalf("placement mismatch = %v, %t; want 0.5, true", mismatch, ok)
	}
	remote, ratio, ok := remoteRatio(100, 60)
	if !ok || remote != 40 || ratio != 0.4 {
		t.Fatalf("remote calculation = %v, %v, %t", remote, ratio, ok)
	}
	if got := badnessScore(mismatch, &ratio); got != 0.5 {
		t.Fatalf("badness = %v, want 0.5", got)
	}
	if got := badnessScore(0.2, &ratio); got != 0.4 {
		t.Fatalf("badness = %v, want 0.4", got)
	}
}

func TestCounterRateResetAndUnavailable(t *testing.T) {
	if rate, ok := counterRate(100, 300, 2*time.Second); !ok || rate != 100 {
		t.Fatalf("rate = %v, %t; want 100, true", rate, ok)
	}
	if _, ok := counterRate(300, 10, time.Second); ok {
		t.Fatal("counter reset should be unavailable")
	}
	if _, ok := counterRate(1, 2, 0); ok {
		t.Fatal("zero-duration rate should be unavailable")
	}
}

func TestParseRangeList(t *testing.T) {
	nodes, err := parseRangeList("0-2,4,7-8\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []int{0, 1, 2, 4, 7, 8}
	if len(nodes) != len(want) {
		t.Fatalf("nodes = %v, want %v", nodes, want)
	}
	for index := range want {
		if nodes[index] != want[index] {
			t.Fatalf("nodes = %v, want %v", nodes, want)
		}
	}
}
