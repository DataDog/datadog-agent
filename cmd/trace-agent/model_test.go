// Some benchmarks defined here because it both requires fixtures & model
// and putting them in model would cause a circular dependency.

package main

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/test/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
)

const (
	defaultEnv = "dev"
)

func BenchmarkHandleSpanRandom(b *testing.B) {
	sb := agent.NewStatsRawBucket(0, 1e9)
	aggr := []string{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		trace := testutil.RandomTrace(10, 8)
		root := traceutil.GetRoot(trace)
		traceutil.ComputeTopLevel(trace)
		wt := agent.NewWeightedTrace(trace, root)
		for _, span := range wt {
			sb.HandleSpan(span, defaultEnv, aggr, nil)
		}
	}
}
