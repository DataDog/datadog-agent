// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ckey

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"testing"

	util "github.com/DataDog/datadog-agent/pkg/util/sort"
	"github.com/stretchr/testify/assert"
)

// Helper function

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"

func generateRandomTags(tagSize, listSize int) []string {
	var tags []string
	for i := 0; i < listSize; i++ {
		t := make([]byte, tagSize)
		for i := range t {
			t[i] = letterBytes[rand.Int63()%int64(len(letterBytes))]
		}
		tags = append(tags, string(t))
	}
	return tags
}

// Unit testing: compare selection sort with stdlib on random slices

func TestSortOK(t *testing.T) {
	for n := 0; n < 10; n++ {
		t.Run(fmt.Sprintf("iteration %d:", n), func(t *testing.T) {
			toSort := generateRandomTags(20, 10)
			selSort := make([]string, len(toSort))
			copy(selSort, toSort)
			stdSort := make([]string, len(toSort))
			copy(stdSort, toSort)

			util.InsertionSort(selSort)
			sort.Strings(stdSort)

			assert.Equal(t, stdSort, selSort)
		})
	}
}

// Benchmark insertion sort vs stdlib
// Run with `go test -bench=. -count=5 -benchmem ./pkg/aggregator/ckey/`
//
// While running these benchmarks, you'll notice that the insertion sort benchmark
// is allocating memory: it comes from its call to generate to re-generate a set
// of tags to sort.
// It's important to re-generate the tags slice between each run of the benchmark
// in order to be sure to not bench the sort against one of its optimal scenario
// (or sub-optimal).
// You should observe the stdlib sort allocate 1 more byte per operation than the
// insertion sort, that's the actual allocation cost of the stdlib sort.

var benchmarkTags []string

func generate() {
	listSize, err := strconv.Atoi(os.Getenv("LISTSIZE"))
	if err != nil {
		listSize = 19
	}
	tagSize, err := strconv.Atoi(os.Getenv("TAGSIZE"))
	if err != nil {
		tagSize = 20
	}
	benchmarkTags = generateRandomTags(tagSize, listSize)
}

func BenchmarkInsertionSort(b *testing.B) {
	t := make([]string, len(benchmarkTags))

	for n := 0; n < b.N; n++ {
		generate()
		copy(t, benchmarkTags)
		util.InsertionSort(t)
	}
}

func BenchmarkStdlibSort(b *testing.B) {
	t := make([]string, len(benchmarkTags))

	for n := 0; n < b.N; n++ {
		generate()
		copy(t, benchmarkTags)
		sort.Strings(t)
	}
}
