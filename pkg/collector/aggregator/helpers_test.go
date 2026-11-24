// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"fmt"
	"strings"
	"testing"
	"unsafe"
)

var (
	stringsSink []string
	stringSink  string
)

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

func BenchmarkCStringArrayToSliceSteadyState(b *testing.B) {
	tags := []string{
		"env:prod",
		"service:payments-api",
		"kube_cluster_name:us-east-1-prod",
		"kube_namespace:payments",
		"pod_name:payments-api-7f9b8c6d5-x2k9p",
		"container_name:app",
		"http_method:POST",
		"http_status:200",
		"le:0.05",
		"version:1.42.0",
	}
	cArray := testHelperSliceToCStringArray(tags)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		stringsSink = CStringArrayToSlice(unsafe.Pointer(cArray))
	}
}

func BenchmarkCStringArrayToSliceChurn(b *testing.B) {
	// 250 distinct tag sets x 10 tags = 2500 unique strings (exceeds 1000-entry cap),
	// modeling OpenMetrics histogram buckets and Kubernetes pod tags.
	const numSets = 250
	const tagsPerSet = 10
	cArrays := make([]unsafe.Pointer, numSets)
	for i := range numSets {
		tags := make([]string, tagsPerSet)
		for j := range tagsPerSet {
			tags[j] = fmt.Sprintf("le:bucket_%03d_series_%03d", j, i)
		}
		cArrays[i] = unsafe.Pointer(testHelperSliceToCStringArray(tags))
	}

	b.ReportAllocs()
	b.ResetTimer()

	i := 0
	for b.Loop() {
		stringsSink = CStringArrayToSlice(cArrays[i%numSets])
		i++
	}
}

func BenchmarkCStringArrayToSliceParallel(b *testing.B) {
	tags := []string{
		"env:prod",
		"service:payments-api",
		"kube_cluster_name:us-east-1-prod",
		"kube_namespace:payments",
		"pod_name:payments-api-7f9b8c6d5-x2k9p",
		"container_name:app",
		"http_method:POST",
		"http_status:200",
		"le:0.05",
		"version:1.42.0",
	}
	cArray := testHelperSliceToCStringArray(tags)

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		var local []string
		for pb.Next() {
			local = CStringArrayToSlice(unsafe.Pointer(cArray))
		}
		_ = local
	})
}

func BenchmarkCStringConvert(b *testing.B) {
	cCheckID := testHelperCString("openmetrics:1a2b3c4d5e6f7a8b")
	cMetricName := testHelperCString("http.server.request.duration.seconds.bucket")
	cHostname := testHelperCString("ip-10-0-42-123.ec2.internal")

	b.Run("C.GoString", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			stringSink = testHelperGoString(cCheckID)
			stringSink = testHelperGoString(cMetricName)
			stringSink = testHelperGoString(cHostname)
		}
	})

	b.Run("cStringToInterned", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			si := getInterner()
			stringSink = cStringToInterned(cCheckID, si)
			stringSink = cStringToInterned(cMetricName, si)
			stringSink = cStringToInterned(cHostname, si)
			putInterner(si)
		}
	})
}
