// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"strings"
	"testing"
	"unsafe"
)

var stringsSink []string

func Benchmark_cStringArrayToSlice(b *testing.B) {
	const numStrings = 10 // fermi estimate
	const stringLen = 10  // fermi estimate
	slice := make([]string, numStrings)
	for i := range slice {
		slice[i] = strings.Repeat("a", stringLen)
	}
	// Note: cArray is not freed, but that's fine for the benchmark.
	cArray := testHelperSliceToCStringArray(slice)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		stringsSink = CStringArrayToSlice(unsafe.Pointer(cArray))
	}
}

func TestInternCStringsSharesStorage(t *testing.T) {
	sharedInternerMu.Lock()
	sharedInterner = newStringInterner()
	sharedInternerMu.Unlock()
	// Note: cArray is not freed, but that's fine for the test.
	cArray := testHelperSliceToCStringArray([]string{"sqlserver.index.user_seeks", "sqlserver-host"})
	cStrings := unsafe.Slice(cArray, 2)

	name1, host1 := internCStrings(cStrings[0], cStrings[1])
	name2, host2 := internCStrings(cStrings[0], cStrings[1])

	if name1 != "sqlserver.index.user_seeks" || host1 != "sqlserver-host" {
		t.Fatalf("got name %q and hostname %q", name1, host1)
	}
	if unsafe.StringData(name1) != unsafe.StringData(name2) || unsafe.StringData(host1) != unsafe.StringData(host2) {
		t.Error("repeated submissions should share the interned name and hostname")
	}
	if name, host := internCStrings(cStrings[0], nil); name != name1 || host != "" {
		t.Errorf("got name %q and hostname %q for a nil hostname", name, host)
	}
}

var nameSink, hostnameSink string

func Benchmark_internCStrings(b *testing.B) {
	// Note: cArray is not freed, but that's fine for the benchmark.
	cArray := testHelperSliceToCStringArray([]string{"sqlserver.index.user_seeks", "sqlserver-host"})
	cStrings := unsafe.Slice(cArray, 2)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		nameSink, hostnameSink = internCStrings(cStrings[0], cStrings[1])
	}
}
