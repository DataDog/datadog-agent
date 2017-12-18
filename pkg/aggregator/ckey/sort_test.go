package ckey

import (
	"fmt"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"testing"

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

			selectionSort(selSort)
			sort.Strings(stdSort)

			assert.Equal(t, stdSort, selSort)
		})
	}

}

// Benchmark selection sort vs stdlib
// Run with `go test -bench=. -benchmem ./pkg/aggregator/ckey/`

var benchmarkTags []string

func init() {
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

func BenchmarkSelectionSort(b *testing.B) {
	t := make([]string, len(benchmarkTags))

	for n := 0; n < b.N; n++ {
		copy(t, benchmarkTags)
		selectionSort(t)
	}
}

func BenchmarkStdlibSort(b *testing.B) {
	t := make([]string, len(benchmarkTags))

	for n := 0; n < b.N; n++ {
		copy(t, benchmarkTags)
		sort.Strings(t)
	}
}
