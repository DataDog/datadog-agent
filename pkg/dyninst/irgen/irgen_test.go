// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package irgen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

func TestCompileUnitFromNameCases(t *testing.T) {
	type testCase struct {
		testName string
		symbol   string
		want     string
	}
	tc := func(symbol, want string) testCase {
		return testCase{
			testName: symbol[:min(len(symbol), 32)],
			symbol:   symbol,
			want:     want,
		}
	}
	testCases := []testCase{
		tc(
			"github.com/DataDog/datadog-agent/pkg/dyninst/irgen.Foo",
			"github.com/DataDog/datadog-agent/pkg/dyninst/irgen",
		),
		tc(
			"a/b.Foo.Bar.Baz",
			"a/b",
		),
		tc(
			"github.com/pkg/errors.(*withStack).Format",
			"github.com/pkg/errors",
		),
		tc("int", "runtime"),
		{
			testName: "long generic type",
			symbol:   "sync/atomic.(*Pointer[go.shape.struct { gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.point gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.statsPoint; gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.kafkaOffset gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.kafkaOffset; gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.typ gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.pointType; gopkg.in/DataDog/dd-trace-go.v1/internal/datastreams.queuePos int64 }]).Swap",
			want:     "sync/atomic",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.testName, func(t *testing.T) {
			require.Equal(t, tc.want, compileUnitFromName(tc.symbol))
		})
	}

}

func TestPickLineInjectionPC(t *testing.T) {
	for _, tc := range []struct {
		name   string
		lines  []line
		ranges []ir.PCRange
		want   uint64
	}{
		{
			name: "inherited matching row",
			lines: []line{
				{pc: 10, line: 9, isStatement: true},
				{pc: 30, line: 10},
			},
			want: 20,
		},
		{
			name:  "inherited final row",
			lines: []line{{pc: 10, line: 9}},
			want:  20,
		},
		{
			name:  "inherited mismatching row",
			lines: []line{{pc: 10, line: 8}},
		},
		{
			name: "superseded matching row",
			lines: []line{
				{pc: 10, line: 9},
				{pc: 15, line: 8},
			},
		},
		{
			name: "row ends at range start",
			lines: []line{
				{pc: 10, line: 9},
				{pc: 20, line: 8},
			},
		},
		{
			name: "gap after sequence end",
			lines: []line{
				{pc: 10, line: 9},
				{pc: 15, line: 9, isStatement: true, endSequence: true},
				{pc: 25, line: 8},
			},
		},
		{
			name: "sequence ends at range start",
			lines: []line{
				{pc: 10, line: 9},
				{pc: 20, line: 9, isStatement: true, endSequence: true},
			},
		},
		{
			name: "sequence ends inside range",
			lines: []line{
				{pc: 10, line: 9},
				{pc: 25, line: 9, isStatement: true, endSequence: true},
			},
			want: 20,
		},
		{
			name: "new sequence at range start",
			lines: []line{
				{pc: 10, line: 8},
				{pc: 20, endSequence: true},
				{pc: 20, line: 9, isStatement: true},
			},
			want: 20,
		},
		{
			name: "superseded row at same address",
			lines: []line{
				{pc: 10, line: 9},
				{pc: 10, line: 8},
			},
		},
		{
			name:  "row starts at range end",
			lines: []line{{pc: 30, line: 9, isStatement: true}},
		},
		{
			name:  "row starts at range start",
			lines: []line{{pc: 20, line: 9, isStatement: true}},
			want:  20,
		},
		{
			name: "prefer statement over inherited row",
			lines: []line{
				{pc: 10, line: 9, isStatement: true},
				{pc: 25, line: 9, isStatement: true},
			},
			want: 25,
		},
		{
			name: "prefer statement in later range",
			lines: []line{
				{pc: 10, line: 9, isStatement: true},
				{pc: 45, line: 9, isStatement: true},
			},
			ranges: []ir.PCRange{{20, 30}, {40, 50}},
			want:   45,
		},
		{
			name: "first nonstatement fallback",
			lines: []line{
				{pc: 10, line: 9},
				{pc: 25, line: 9},
			},
			want: 20,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ranges := tc.ranges
			if ranges == nil {
				ranges = []ir.PCRange{{20, 30}}
			}
			root := ir.PCRange{10, 60}
			pc, issue, err := pickLineInjectionPC(9, ranges, []ir.PCRange{root}, map[ir.PCRange]lineData{
				root: {lines: tc.lines},
			})
			require.NoError(t, err)
			if tc.want == 0 {
				require.Equal(t, ir.IssueKindInvalidProbeDefinition, issue.Kind)
			} else {
				require.True(t, issue.IsNone(), "%v", issue)
			}
			require.Equal(t, tc.want, pc)
		})
	}
}
