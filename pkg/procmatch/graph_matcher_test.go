package procmatch

import "testing"

// Benchmarks
var avoidOptimizationGraph string

func BenchmarkGraph10(b *testing.B) {
	m, err := NewWithGraph(rawIntegs)
	if err != nil {
		return
	}
	benchmarkGraph(b, m)
}

func BenchmarkGraph20(b *testing.B) {
	m, err := NewWithGraph(append(rawIntegs, rawIntegs2...))
	if err != nil {
		return
	}
	benchmarkGraph(b, m)
}

func BenchmarkGraph40(b *testing.B) {
	m, err := NewWithGraph(append(append(rawIntegs, rawIntegs2...), rawIntegs3...))
	if err != nil {
		return
	}
	benchmarkGraph(b, m)
}

func benchmarkGraph(b *testing.B, m Matcher) {
	test := "myprogram -Xmx4000m -Xms4000m -XX:ReservedCodeCacheSize=256m -port 9999 kafka.Kafka"

	var r string
	for n := 0; n < b.N; n++ {
		r = m.Match(test)
	}
	avoidOptimizationGraph = r
}
