// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package darwin

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/darwin/eslogger"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/process"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// These benchmarks measure the collector-side cost of the pipeline: decode ->
// translate -> evaluate. That is the part of the overhead budget this code
// controls. The Endpoint Security subscription itself has a cost too, but it is
// Apple's and can only be measured with the collector actually attached.
//
// The question they answer is whether the pipeline can keep up with the volume a
// subscription produces, which matters most for `open`: Endpoint Security can mute
// paths but cannot subscribe to only some of them, so an open subscription sees
// everything the machine does.

// syntheticStream builds a JSON-Lines stream shaped like real eslogger output.
func syntheticStream(b *testing.B, events int, includeOpens bool) []byte {
	b.Helper()

	var buf bytes.Buffer
	pid := uint32(10000)
	for i := 0; i < events; i++ {
		pid++
		// A realistic unit of activity: fork, exec, some opens, exit.
		fmt.Fprintf(&buf, `{"time":"2026-08-12T00:00:00Z","global_seq_num":%d,"event":{"fork":{"child":{"executable":{"path":"/bin/zsh"},"ppid":1,"audit_token":{"pid":%d,"pidversion":%d,"ruid":501,"euid":501,"rgid":20,"egid":20}}}}}`+"\n",
			i*4, pid, pid)
		fmt.Fprintf(&buf, `{"time":"2026-08-12T00:00:00Z","global_seq_num":%d,"event":{"exec":{"target":{"executable":{"path":"/usr/local/bin/npm"},"ppid":1,"audit_token":{"pid":%d,"pidversion":%d,"ruid":501,"euid":501,"rgid":20,"egid":20}},"args":["npm","install","--no-audit"],"env":["PATH=/usr/bin","HOME=/Users/x"]}}}`+"\n",
			i*4+1, pid, pid+1)
		if includeOpens {
			fmt.Fprintf(&buf, `{"time":"2026-08-12T00:00:00Z","global_seq_num":%d,"process":{"audit_token":{"pid":%d}},"event":{"open":{"fflag":0,"file":{"path":"/Users/x/proj/node_modules/pkg/index.js"}}}}`+"\n",
				i*4+2, pid)
		}
		fmt.Fprintf(&buf, `{"time":"2026-08-12T00:00:00Z","global_seq_num":%d,"process":{"audit_token":{"pid":%d}},"event":{"exit":{"stat":0}}}`+"\n",
			i*4+3, pid)
	}
	return buf.Bytes()
}

func benchPipeline(b *testing.B, includeOpens bool) {
	b.Helper()

	const units = 250
	stream := syntheticStream(b, units, includeOpens)

	scrubber, err := utils.NewScrubber(nil, nil)
	require.NoError(b, err)

	b.ReportAllocs()
	b.SetBytes(int64(len(stream)))
	b.ResetTimer()

	var decoded int
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// A fresh resolver per iteration: otherwise the process cache grows without
		// bound across iterations and the numbers measure the cache, not the work.
		pr, err := process.NewEBPFLessResolver(nil, nil, scrubber, process.NewResolverOpts())
		require.NoError(b, err)
		tr := NewTranslator(pr, NewFieldHandlers(pr, "bench"))
		rs, err := NewRuleSet("policies", func() eval.Event { return tr.newEvent() })
		require.NoError(b, err)
		rs.AddListener(&MatchRecorder{})
		d := eslogger.NewDecoder(bytes.NewReader(stream))
		b.StartTimer()

		for {
			msg, err := d.Next()
			if err != nil {
				break
			}
			ev, err := tr.Translate(msg)
			if err != nil || ev == nil {
				continue
			}
			rs.Evaluate(ev)
			decoded++
		}
	}

	b.StopTimer()
	if b.N > 0 {
		b.ReportMetric(float64(decoded)/float64(b.N), "events/iter")
	}
}

// BenchmarkPipelineProcessOnly is the exec/fork/exit subscription: the one the PoC
// runs by default.
func BenchmarkPipelineProcessOnly(b *testing.B) {
	benchPipeline(b, false)
}

// BenchmarkPipelineWithOpens adds an open per unit of activity, which is the
// subscription expected to dominate volume.
func BenchmarkPipelineWithOpens(b *testing.B) {
	benchPipeline(b, true)
}

// BenchmarkDecodeOnly separates JSON decoding from translation and evaluation, so
// it is clear which half any cost sits in.
func BenchmarkDecodeOnly(b *testing.B) {
	stream := syntheticStream(b, 250, true)

	b.ReportAllocs()
	b.SetBytes(int64(len(stream)))
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		d := eslogger.NewDecoder(bytes.NewReader(stream))
		for {
			if _, err := d.Next(); err != nil {
				break
			}
		}
	}
}
