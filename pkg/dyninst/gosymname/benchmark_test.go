// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosymname

import "testing"

var benchSymbols = []struct {
	name   string
	input  string
	source SymbolSource
}{
	{"simple_func", "encoding/json.Marshal", SourcePclntab},
	{"ptr_receiver", "github.com/cockroachdb/cockroach/pkg/kv/kvserver.(*raftSchedulerShard).worker", SourceDWARF},
	{"closure", "github.com/getsentry/sentry-go.NewClient.func1", SourceDWARF},
	{"deep_chain", "crypto/tls.keysFromMasterSecret.prfForVersion.prfAndHashForVersion.prf12.func2", SourcePclntab},
	{"generic", "github.com/cockroachdb/cockroach/pkg/kv/kvclient/kvcoord.(*rangeFeedRegistry).ForEachPartialRangefeed.(*Set[go.shape.*github.com/cockroachdb/cockroach/pkg/kv/kvclient/kvcoord.activeRangeFeed]).Range.func3", SourcePclntab},
	{"escaped", "gopkg.in/square/go-jose%2ev2.newBuffer", SourceDWARF},
}

func BenchmarkParse(b *testing.B) {
	for _, bs := range benchSymbols {
		b.Run(bs.name, func(b *testing.B) {
			for b.Loop() {
				Parse(bs.input, bs.source)
			}
		})
	}
}

func BenchmarkParsePackageOnly(b *testing.B) {
	for _, bs := range benchSymbols {
		b.Run(bs.name, func(b *testing.B) {
			for b.Loop() {
				s := Parse(bs.input, bs.source)
				_ = s.Package()
			}
		})
	}
}

func BenchmarkParseFullInterpretation(b *testing.B) {
	for _, bs := range benchSymbols {
		b.Run(bs.name, func(b *testing.B) {
			for b.Loop() {
				s := Parse(bs.input, bs.source)
				_ = s.Interpretations()
			}
		})
	}
}

func BenchmarkParseInto(b *testing.B) {
	for _, bs := range benchSymbols {
		b.Run(bs.name, func(b *testing.B) {
			var s Symbol
			for b.Loop() {
				ParseInto(&s, bs.input, bs.source)
			}
		})
	}
}

func BenchmarkParseAndMaterialize(b *testing.B) {
	for _, bs := range benchSymbols {
		b.Run(bs.name, func(b *testing.B) {
			for b.Loop() {
				s := Parse(bs.input, bs.source)
				interps := s.Interpretations()
				for i := range interps {
					_ = interps[i].QualifiedName()
					_ = interps[i].IsMethod()
					_ = interps[i].IsGeneric()
					_ = interps[i].HasInlinedCalls()
					_ = interps[i].BaseName()
					for j := range interps[i].InlinedCalls {
						_ = interps[i].InlinedCalls[j].QualifiedFunction()
					}
				}
			}
		})
	}
}

func BenchmarkSplitPackage(b *testing.B) {
	for _, bs := range benchSymbols {
		b.Run(bs.name, func(b *testing.B) {
			for b.Loop() {
				SplitPackage(bs.input, bs.source)
			}
		})
	}
}
