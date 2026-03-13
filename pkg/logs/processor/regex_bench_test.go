// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

var crossoverSizes = []int{64, 256, 1024, 4096, 16384, 65536, 262144}

var crossoverPatterns = []struct {
	name    string
	pattern string
	repl    string
}{
	{"literal_prefix", `api_key=[a-f0-9]{28}`, "[REDACTED]"},
	{"simple_alternation", `(?:secret|token|apikey)=[a-f0-9]+`, "[REDACTED]"},
	{"complex_alternation", `(?:api[_-]?key|secret[_-]?token|auth[_-]?bearer|password|credentials|private[_-]?key|access[_-]?token|refresh[_-]?token)\s*[=:]\s*[A-Za-z0-9+/=_-]{16,}`, "[REDACTED]"},
	{"credit_card", `(?:4[0-9]{12}(?:[0-9]{3})?|[25][1-7][0-9]{14}|6(?:011|5[0-9][0-9])[0-9]{12}|3[47][0-9]{13}|3(?:0[0-5]|[68][0-9])[0-9]{11}|(?:2131|1800|35\d{3})\d{11})`, "[REDACTED_CC]"},
}

func buildCrossoverContent(size int) []byte {
	const filler = "2025-03-09 INFO handler.go request_id=abc123 processed in 42ms "
	reps := (size / len(filler)) + 1
	return []byte(strings.Repeat(filler, reps)[:size])
}

// BenchmarkCrossover measures re2MaskReplace across a matrix of message sizes
// and pattern complexities. Run under different build tags (stdlib vs go-re2)
// and compare with benchstat to find the crossover point.
func BenchmarkCrossover(b *testing.B) {
	for _, pat := range crossoverPatterns {
		for _, size := range crossoverSizes {
			name := fmt.Sprintf("%s/%dB", pat.name, size)
			content := buildCrossoverContent(size)
			rule := newProcessingRule(config.MaskSequences, pat.repl, pat.pattern)
			b.Run(name, func(b *testing.B) {
				b.SetBytes(int64(size))
				b.ResetTimer()
				for range b.N {
					re2MaskReplace(rule, content)
				}
			})
		}
	}
}

// BenchmarkMatchCrossover measures re2MatchContent (the exclude/include path)
// across the same size x pattern matrix used for mask. This isolates the
// Match-only cost to determine whether routing exclude/include rules through
// RE2's DFA would be beneficial on large content with complex patterns.
func BenchmarkMatchCrossover(b *testing.B) {
	for _, pat := range crossoverPatterns {
		for _, size := range crossoverSizes {
			name := fmt.Sprintf("%s/%dB", pat.name, size)
			content := buildCrossoverContent(size)
			rule := newProcessingRule(config.ExcludeAtMatch, "", pat.pattern)
			b.Run(name, func(b *testing.B) {
				b.SetBytes(int64(size))
				b.ResetTimer()
				for range b.N {
					re2MatchContent(rule, content)
				}
			})
		}
	}
}
