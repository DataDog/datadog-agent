// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package jsonprune

import (
	"fmt"
	"strings"
	"testing"
)

// makeWideLeaves produces a snapshot with n simple captured-value
// leaves at level 6 (one level deeper than arguments).
func makeWideLeaves(n, leafSize int) []byte {
	args := map[string]string{}
	for i := 0; i < n; i++ {
		args[fmt.Sprintf("v%d", i)] = fmt.Sprintf(
			`{"type":"T","data":%q}`, strings.Repeat("x", leafSize))
	}
	return []byte(envelope(args))
}

// makeDeepChain produces a pointer-chain value (depth nested objects,
// each wrapping the next in a "next" field). Models the bpf continuation
// shape that motivates pruning.
func makeDeepChain(depth, leafSize int) []byte {
	val := fmt.Sprintf(`{"type":"Leaf","data":%q}`,
		strings.Repeat("x", leafSize))
	for i := depth - 1; i >= 0; i-- {
		val = fmt.Sprintf(`{"type":"Node","next":%s}`, val)
	}
	return []byte(envelope(map[string]string{"chain": val}))
}

// makeDeepTree produces a balanced tree of the given fan-out and depth.
func makeDeepTree(fanout, depth int) []byte {
	var build func(int) string
	build = func(d int) string {
		if d == 0 {
			return `{"type":"Leaf","data":"payload-data"}`
		}
		var b strings.Builder
		b.WriteString(`{"type":"Inner","children":{`)
		for i := 0; i < fanout; i++ {
			if i > 0 {
				b.WriteByte(',')
			}
			fmt.Fprintf(&b, `"c%d":%s`, i, build(d-1))
		}
		b.WriteString("}}")
		return b.String()
	}
	return []byte(envelope(map[string]string{"tree": build(depth)}))
}

func BenchmarkPruneFastPath(b *testing.B) {
	input := makeWideLeaves(10, 50)
	budget := len(input) * 2
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for i := 0; i < b.N; i++ {
		_ = Prune(input, budget)
	}
}

func BenchmarkPruneWideLeaves(b *testing.B) {
	input := makeWideLeaves(1000, 80)
	// Trim by 30% so many leaves must be pruned.
	budget := len(input) * 7 / 10
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for i := 0; i < b.N; i++ {
		_ = Prune(input, budget)
	}
}

func BenchmarkPruneDeepChain(b *testing.B) {
	input := makeDeepChain(500, 40)
	budget := len(input) / 3
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for i := 0; i < b.N; i++ {
		_ = Prune(input, budget)
	}
}

func BenchmarkPruneDeepTree(b *testing.B) {
	input := makeDeepTree(4, 6) // 4^6 = 4096 leaves
	budget := len(input) / 3
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for i := 0; i < b.N; i++ {
		_ = Prune(input, budget)
	}
}

func BenchmarkPruneBarelyOver(b *testing.B) {
	input := makeWideLeaves(20, 100)
	budget := len(input) - 1
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for i := 0; i < b.N; i++ {
		_ = Prune(input, budget)
	}
}

func BenchmarkPrunePoolReuse(b *testing.B) {
	input := makeWideLeaves(100, 80)
	budget := len(input) * 7 / 10
	// Warm the pool.
	_ = Prune(input, budget)
	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for i := 0; i < b.N; i++ {
		_ = Prune(input, budget)
	}
}
