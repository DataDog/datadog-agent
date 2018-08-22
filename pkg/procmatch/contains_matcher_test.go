package procmatch

import "testing"

// Benchmarks
var avoidOptimizationContains string

func BenchmarkContains10(b *testing.B) {
	m := NewWithContains(rawIntegs)
	benchmarkContains(b, m)
}

func BenchmarkContains20(b *testing.B) {
	m := NewWithContains(append(rawIntegs, rawIntegs2...))
	benchmarkContains(b, m)
}

func BenchmarkContains40(b *testing.B) {
	m := NewWithContains(append(append(rawIntegs, rawIntegs2...), rawIntegs3...))
	benchmarkContains(b, m)
}

func benchmarkContains(b *testing.B, m Matcher) {
	test := "myprogram -Xmx4000m -Xms4000m -XX:ReservedCodeCacheSize=256m -port 9999 kafka.Kafka"

	var r string
	for n := 0; n < b.N; n++ {
		r = m.Match(test)
	}
	avoidOptimizationContains = r
}
