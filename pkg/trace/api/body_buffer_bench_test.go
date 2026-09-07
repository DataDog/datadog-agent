// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"math/rand"
	"net/http"
	"testing"
)

// BenchmarkCopyRequestBody measures the cost of buffering a request body on the
// real path, including the sync.Pool, across payload-size regimes.
func BenchmarkCopyRequestBody(b *testing.B) {
	// Sizes are calibrated to the staging profile. Splitting alloc_space by
	// alloc_objects there shows only ~1400 buffer allocations in 300s, averaging
	// 5.3MB at the Grow site and 15.4MB at the ReadFrom site. So this is not a
	// per-request cost: ordinary traffic is absorbed by the pool, and the 11GB
	// comes from a small number of very large payloads.
	rng := rand.New(rand.NewSource(1))
	small := make([]int, 64)
	for i := range small {
		small[i] = 2<<10 + rng.Intn(126<<10) // 2KiB..128KiB, ordinary submissions
	}
	large := []int{5 << 20} // 5MiB, the average size at the Grow site

	cases := []struct {
		name  string
		sizes []int
		pool  bool
	}{
		// The common case: the pool converges and nothing reallocates. Included
		// to show the fix is neutral here rather than a regression.
		{"SmallWarmPool", small, true},
		// The case that actually generates the 11GB. A multi-MB body arrives and
		// no pooled buffer is anywhere near large enough, so Grow(n) allocates n
		// and ReadFrom then reallocates to ~2n: ~3n allocated to move n bytes.
		// Modeled without pool reuse because at 6.5 GC/s a large buffer rarely
		// survives in the pool until the next large payload ~0.2s later.
		{"LargeColdBuffer", large, false},
	}

	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			r := testReceiver(50 << 20)
			reqs := make([]*http.Request, len(tc.sizes))
			bodies := make([]*bodyReader, len(tc.sizes))
			payloads := make([][]byte, len(tc.sizes))
			total := 0
			for i, sz := range tc.sizes {
				payloads[i] = bytes.Repeat([]byte("x"), sz)
				reqs[i], bodies[i] = newBodyRequest(payloads[i])
				total += sz
			}
			b.SetBytes(int64(total / len(tc.sizes)))
			b.ReportAllocs()

			i := 0
			for b.Loop() {
				idx := i % len(reqs)
				i++
				bodies[idx].r.Reset(payloads[idx])

				var buf *bytes.Buffer
				if tc.pool {
					buf = getBuffer()
				} else {
					buf = new(bytes.Buffer)
				}
				n, err := r.copyRequestBody(buf, reqs[idx])
				if err != nil {
					b.Fatal(err)
				}
				if int(n) != len(payloads[idx]) {
					b.Fatalf("short copy: got %d want %d", n, len(payloads[idx]))
				}
				if tc.pool {
					putBuffer(buf)
				}
			}
		})
	}
}
