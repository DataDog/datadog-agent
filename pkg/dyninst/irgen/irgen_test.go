// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package irgen

import (
	"debug/dwarf"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
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

func TestFollowLineJumpGenericContainsLoop(t *testing.T) {
	// ARM64 genericContains[go.shape.int], built with Go 1.27.3.
	body, err := hex.DecodeString("e10b00f9e3031faa02000014630400915f0003ebcd000054257863f89f0005eb61ffff54e00340b2c0035fd6e0031faac0035fd6")
	require.NoError(t, err)
	const addr = uint64(0x100)
	file := &dwarf.LineFile{Name: "generics.go"}
	lines := []line{
		{pc: addr, line: 79, file: file},
		{pc: addr + 4, line: 80, isStatement: true, file: file},
		{pc: addr + 24, line: 81, file: file},
		{pc: addr + 36, line: 82, file: file},
		{pc: addr + 44, line: 85, file: file},
	}
	require.Equal(t, addr+4, followLineJump("arm64", addr, addr+4, body, lines))
}

func TestFollowLineJump(t *testing.T) {
	file := &dwarf.LineFile{Name: "example.go"}
	otherFile := &dwarf.LineFile{Name: "other.go"}
	const addr = uint64(0x100)
	for _, arch := range []struct {
		name                               object.Architecture
		body, conditional, call, backward  string
		loopBack, loopHeader, loopOutside  string
		secondMove, afterJump, afterTarget uint64
	}{
		// Two moves, a forward jump, padding, and a return.
		{"amd64", "488b4424084889c3e9070000009090909090909090c3", "7512", "e80f000000", "e9f3ffffff", "75f7", "75fe", "75ea", 5, 13, 21},
		{"arm64", "e00301aae10302aa030000141f2003d51f2003d51f2003d5c0035fd6", "a0000054", "05000094", "feffff17", "c1ffff54", "00000014", "61ffff54", 4, 12, 24},
	} {
		t.Run(string(arch.name), func(t *testing.T) {
			decode := func(s string) []byte {
				b, err := hex.DecodeString(s)
				require.NoError(t, err)
				return b
			}
			for _, tc := range []struct {
				name   string
				change func([]byte, []line) []byte
				want   uint64
			}{
				{"shared line", nil, addr + 20},
				{"different file", func(b []byte, l []line) []byte { l[4].file = otherFile; return b }, addr},
				{"different line", func(b []byte, l []line) []byte { l[4].line++; return b }, addr},
				{"sequence end", func(b []byte, l []line) []byte { l[4].endSequence = true; return b }, addr},
				{"crosses another line", func(b []byte, l []line) []byte { l[1].line--; return b }, addr},
				{"conditional branch", func(b []byte, _ []line) []byte { copy(b, decode(arch.conditional)); return b }, addr},
				{"call", func(b []byte, _ []line) []byte { copy(b, decode(arch.call)); return b }, addr},
				{"backward jump", func(b []byte, _ []line) []byte { copy(b[8:], decode(arch.backward)); return b }, addr},
				{"loop back edge", func(b []byte, _ []line) []byte { return append(b[:20], decode(arch.loopBack)...) }, addr},
				{"loop header", func(b []byte, _ []line) []byte { return append(b[:20], decode(arch.loopHeader)...) }, addr},
				{"loop through original probe", func(b []byte, _ []line) []byte { return append(b[:20], decode(arch.loopOutside)...) }, addr + 20},
				{"zero padding", func(b []byte, _ []line) []byte { return append(b, 0, 0, 0, 0) }, addr + 20},
				{"outside function", func(b []byte, _ []line) []byte { return b[:20] }, addr},
				{"truncated instruction", func(b []byte, _ []line) []byte { return b[:1] }, addr},
			} {
				t.Run(tc.name, func(t *testing.T) {
					body := decode(arch.body)
					lines := []line{
						{pc: addr, line: 93, isStatement: true, file: file},
						{pc: addr + arch.secondMove, line: 93, file: file},
						{pc: addr + 8, line: 84, file: file},
						{pc: addr + arch.afterJump, line: 86, file: file},
						{pc: addr + 20, line: 93, file: file},
						{pc: addr + arch.afterTarget, line: 94, file: file},
					}
					if tc.change != nil {
						body = tc.change(body, lines)
					}
					require.Equal(t, tc.want, followLineJump(arch.name, addr, addr, body, lines))
				})
			}
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
