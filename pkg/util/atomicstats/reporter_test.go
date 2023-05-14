// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package atomicstats

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
)

func Example() {
	// define a struct with some `stats` tags
	type myStats struct {
		integer       int64         `stats:""`
		atomicInteger *atomic.Int64 `stats:""`
		notStats      int64
	}

	// create a myStats value
	stats := myStats{
		integer:       10,
		atomicInteger: atomic.NewInt64(20),
		notStats:      30,
	}
	statsMap := Report(&stats)

	fmt.Printf("%#v\n", statsMap)
	// Output:
	// map[string]interface {}{"atomic_integer":20, "integer":10}
}

func TestReport_NotPtr(t *testing.T) {
	type myStats struct{}
	require.Panics(t, func() { Report(myStats{}) })
}

func TestReport_NotStructPtr(t *testing.T) {
	someNumber := 13
	require.Panics(t, func() { Report(&someNumber) })
}

func TestReport_BadType(t *testing.T) {
	//nolint:structcheck,unused
	type myStats struct {
		// (if and when we support strings, think of something more interesting)
		stringStat string `stats:""`
	}
	require.Panics(t, func() { Report(&myStats{}) })
}

func TestReporterAllowedTypes(t *testing.T) {
	//nolint:structcheck,unused
	type test struct {
		i64 int64 `stats:""`
		i32 int32 `stats:""`
		i16 int16 `stats:""`
		i8  int8  `stats:""`
		i   int   `stats:""`

		u64 uint64 `stats:""`
		u32 uint32 `stats:""`
		u16 uint16 `stats:""`
		u8  uint8  `stats:""`
		u   uint   `stats:""`

		uptr uintptr `stats:""`
	}

	stats := Report(&test{})
	require.Len(t, stats, 11)
	require.Equal(t, int64(0), stats["i64"])
	require.Equal(t, int32(0), stats["i32"])
	require.Equal(t, int16(0), stats["i16"])
	require.Equal(t, int8(0), stats["i8"])
	require.Equal(t, int(0), stats["i"])

	require.Equal(t, uint64(0), stats["u64"])
	require.Equal(t, uint32(0), stats["u32"])
	require.Equal(t, uint16(0), stats["u16"])
	require.Equal(t, uint8(0), stats["u8"])
	require.Equal(t, uint(0), stats["u"])

	require.Equal(t, uintptr(0), stats["uptr"])
}

func TestReporterSnakeCase(t *testing.T) {
	//nolint:structcheck,unused
	type test struct {
		foo       int `stats:""`
		barBaz    int `stats:""`
		barbaz    int `stats:""`
		fooBarBaz int `stats:""`
	}
	stats := Report(&test{})
	require.Len(t, stats, 4)
	require.Contains(t, stats, "foo")
	require.Contains(t, stats, "bar_baz")
	require.Contains(t, stats, "barbaz")
	require.Contains(t, stats, "foo_bar_baz")
}

func TestReporterSkipNoTag(t *testing.T) {
	//nolint:structcheck,unused
	type test struct {
		foo int `stats:""`
		bar int
		baz int `stats:""`
	}

	stats := Report(&test{})
	require.Len(t, stats, 2)
	require.Contains(t, stats, "foo")
	require.Contains(t, stats, "baz")
	require.NotContains(t, stats, "bar")
}

func TestReporterAllowedTypesAtomic(t *testing.T) {
	//nolint:structcheck,unused
	type test struct {
		boolp *atomic.Bool   `stats:""`
		i64p  *atomic.Int64  `stats:""`
		u64p  *atomic.Uint64 `stats:""`
	}

	v := &test{
		boolp: atomic.NewBool(true),
		i64p:  atomic.NewInt64(6),
		u64p:  atomic.NewUint64(7),
	}
	stats := Report(v)
	require.Len(t, stats, 3)
	require.Equal(t, true, stats["boolp"])
	require.Equal(t, int64(6), stats["i64p"])
	require.Equal(t, uint64(7), stats["u64p"])
}
