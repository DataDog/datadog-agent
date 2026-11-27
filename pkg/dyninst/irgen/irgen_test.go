// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package irgen

import (
	"bytes"
	"debug/dwarf"
	"encoding/hex"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
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
	mainFile := &dwarf.LineFile{Name: "example.com/app/main.go"}
	otherFile := &dwarf.LineFile{Name: "example.com/app/other/main.go"}
	buildFile := &dwarf.LineFile{Name: "/build/example.com/app/main.go"}
	rootedFile := &dwarf.LineFile{Name: "/example.com/app/main.go"}
	for _, tc := range []struct {
		name       string
		sourceFile string
		// declFile defaults to mainFile; "-" means unknown.
		declFile string
		lines    []line
		ranges   []ir.PCRange
		want     uint64
		message  string
	}{
		{
			name:       "same-named inlined file before declaring file",
			sourceFile: "main.go",
			lines: []line{
				{pc: 20, line: 9, isStatement: true, file: otherFile},
				{pc: 25, line: 9, isStatement: true},
			},
			want: 25,
		},
		{
			name:       "exact file before declaring file named by a suffix",
			sourceFile: buildFile.Name,
			lines: []line{
				{pc: 20, line: 9, isStatement: true, file: buildFile},
				{pc: 25, line: 9, isStatement: true},
			},
			want: 20,
		},
		{
			name:       "exact declaring file before file holding its path",
			sourceFile: mainFile.Name,
			lines: []line{
				{pc: 20, line: 9, isStatement: true, file: buildFile},
				{pc: 25, line: 9, isStatement: true},
			},
			want: 25,
		},
		{
			name:       "recorded name with backslashes",
			sourceFile: `C:\src\app\main.go`,
			lines: []line{
				{pc: 20, line: 9, isStatement: true, file: &dwarf.LineFile{Name: `C:\src\app\main.go`}},
				{pc: 25, line: 9, isStatement: true},
			},
			want: 20,
		},
		{
			name:       "recorded names differing only in separators",
			sourceFile: `example.com\app\main.go`,
			lines: []line{
				{pc: 20, line: 9, isStatement: true},
				{pc: 25, line: 9, isStatement: true, file: &dwarf.LineFile{Name: `example.com\app\main.go`}},
			},
			want: 25,
		},
		{
			name:       "recorded name with backslashes, suffix",
			sourceFile: "gen/main.go",
			lines: []line{
				{pc: 20, line: 9, isStatement: true, file: &dwarf.LineFile{Name: `example.com\gen\main.go`}},
				{pc: 25, line: 9, isStatement: true},
			},
			want: 20,
		},
		{
			name:       "rooted file distinct from relative file",
			sourceFile: "/" + mainFile.Name,
			lines: []line{
				{pc: 20, line: 9, isStatement: true},
				{pc: 25, line: 9, isStatement: true, file: rootedFile},
			},
			want: 25,
		},
		{
			name:       "relative file distinct from rooted file",
			sourceFile: mainFile.Name,
			lines: []line{
				{pc: 20, line: 9, isStatement: true, file: rootedFile},
				{pc: 25, line: 9, isStatement: true},
			},
			want: 25,
		},
		{
			name:       "same-named inlined file only",
			sourceFile: "main.go",
			lines:      []line{{pc: 20, line: 9, isStatement: true, file: otherFile}},
			message:    `no code for line 9 of source file "main.go" in the method`,
		},
		{
			name:       "source file names another file of the method",
			sourceFile: "other/main.go",
			lines: []line{
				{pc: 20, line: 9, isStatement: true, file: otherFile},
				{pc: 25, line: 9, isStatement: true},
			},
			want: 20,
		},
		{
			name:       "declaring file unknown, source file matches two files",
			sourceFile: "main.go",
			declFile:   "-",
			lines: []line{
				{pc: 20, line: 9, isStatement: true, file: otherFile},
				{pc: 25, line: 9, isStatement: true},
			},
			message: `source file "main.go" matches more than one file with code at line 9: ` +
				`["example.com/app/main.go" "example.com/app/other/main.go"]`,
		},
		{
			name:       "declaring file unknown, only a superseded row in file",
			sourceFile: "main.go",
			declFile:   "-",
			lines: []line{
				{pc: 10, line: 9},
				{pc: 15, line: 8},
			},
			message: "no suitable injection point found for line: 9",
		},
		{
			name:       "declaring file unknown, superseded row in another file",
			sourceFile: "main.go",
			declFile:   "-",
			lines: []line{
				{pc: 10, line: 9, file: otherFile},
				{pc: 15, line: 8},
				{pc: 25, line: 9, isStatement: true},
			},
			want: 25,
		},
		{
			name:     "declaring file unknown, row in another file",
			declFile: "-",
			lines: []line{
				{pc: 20, line: 9, isStatement: true, file: otherFile},
				{pc: 25, line: 9, isStatement: true},
			},
			want: 25,
		},
		{
			name:    "row in another file",
			lines:   []line{{pc: 20, line: 9, isStatement: true, file: otherFile}},
			message: `no code for line 9 of source file "app/main.go" in the method`,
		},
		{
			name: "superseded row in file and row in another file",
			lines: []line{
				{pc: 10, line: 9},
				{pc: 15, line: 8},
				{pc: 22, line: 9, isStatement: true, file: otherFile},
			},
			message: `no code for line 9 of source file "app/main.go" in the method`,
		},
		{
			name:       "non-canonical source file",
			sourceFile: "app/main.go/x/..",
			lines:      []line{{pc: 20, line: 9, isStatement: true}},
			message:    `invalid source file: "app/main.go/x/.."`,
		},
		{
			name:       "windows separators",
			sourceFile: `app\main.go`,
			lines:      []line{{pc: 20, line: 9, isStatement: true}},
			want:       20,
		},
		{
			name: "skip row in another file",
			lines: []line{
				{pc: 20, line: 9, isStatement: true, file: otherFile},
				{pc: 25, line: 9, isStatement: true},
			},
			want: 25,
		},
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
			lines := slices.Clone(tc.lines)
			for i := range lines {
				if lines[i].file == nil {
					lines[i].file = mainFile
				}
			}
			sourceFile := tc.sourceFile
			if sourceFile == "" {
				sourceFile = "app/main.go"
			}
			declFile := tc.declFile
			switch declFile {
			case "":
				declFile = mainFile.Name
			case "-":
				declFile = ""
			}
			root := ir.PCRange{10, 60}
			rootRanges := []ir.PCRange{root}
			data := map[ir.PCRange]lineData{root: {lines: lines}}
			sp := &ir.Subprogram{InlinePCRanges: []ir.InlinePCRanges{{Ranges: ranges, RootRanges: rootRanges}}}
			var pc uint64
			target := resolveTestLineFile(sourceFile, []string{declFile}, []*ir.Subprogram{sp}, data)
			issue := target.issue
			if issue.IsNone() {
				var err error
				pc, issue, err = pickLineInjectionPC(9, sourceFile, target.file, ranges, rootRanges, data)
				require.NoError(t, err)
			}
			if tc.want == 0 {
				require.Equal(t, ir.IssueKindInvalidProbeDefinition, issue.Kind)
				if tc.message != "" {
					require.Equal(t, tc.message, issue.Message)
				}
			} else {
				require.True(t, issue.IsNone(), "%v", issue)
			}
			require.Equal(t, tc.want, pc)
		})
	}
}

func TestLineProbeFileAcrossInstances(t *testing.T) {
	mainFile := &dwarf.LineFile{Name: "example.com/app/main.go"}
	otherFile := &dwarf.LineFile{Name: "example.com/app/other/main.go"}
	rootA, rootB := ir.PCRange{10, 60}, ir.PCRange{100, 160}
	data := map[ir.PCRange]lineData{
		rootA: {lines: []line{{pc: 20, line: 9, isStatement: true, file: otherFile}}},
		rootB: {lines: []line{{pc: 120, line: 9, isStatement: true, file: mainFile}}},
	}
	instanceA := ir.InlinePCRanges{Ranges: []ir.PCRange{{20, 30}}, RootRanges: []ir.PCRange{rootA}}
	instanceB := ir.InlinePCRanges{Ranges: []ir.PCRange{{120, 130}}, RootRanges: []ir.PCRange{rootB}}

	t.Run("inlined instances of one subprogram", func(t *testing.T) {
		sp := &ir.Subprogram{InlinePCRanges: []ir.InlinePCRanges{instanceA, instanceB}}
		target := resolveTestLineFile("main.go", []string{""}, []*ir.Subprogram{sp}, data)
		require.Contains(t, target.issue.Message, "matches more than one file")
	})
	t.Run("subprograms of one probe", func(t *testing.T) {
		spA := &ir.Subprogram{InlinePCRanges: []ir.InlinePCRanges{instanceA}}
		spB := &ir.Subprogram{InlinePCRanges: []ir.InlinePCRanges{instanceB}}
		target := resolveTestLineFile("main.go", []string{"", ""}, []*ir.Subprogram{spA, spB}, data)
		require.Contains(t, target.issue.Message, "matches more than one file")
	})
	t.Run("declaring file of one subprogram decides for the others", func(t *testing.T) {
		spA := &ir.Subprogram{InlinePCRanges: []ir.InlinePCRanges{instanceA, instanceB}}
		spB := &ir.Subprogram{InlinePCRanges: []ir.InlinePCRanges{instanceB}}
		target := resolveTestLineFile("main.go", []string{"", mainFile.Name}, []*ir.Subprogram{spA, spB}, data)
		require.True(t, target.issue.IsNone(), "%v", target.issue)
		require.Equal(t, mainFile.Name, target.file)
	})
	t.Run("superseded row does not count toward ambiguity", func(t *testing.T) {
		rootC := ir.PCRange{200, 260}
		withSuperseded := maps.Clone(data)
		withSuperseded[rootC] = lineData{lines: []line{{pc: 210, line: 9, file: mainFile}, {pc: 215, line: 8, file: mainFile}}}
		spA := &ir.Subprogram{InlinePCRanges: []ir.InlinePCRanges{
			{Ranges: []ir.PCRange{{220, 230}}, RootRanges: []ir.PCRange{rootC}},
		}}
		spB := &ir.Subprogram{InlinePCRanges: []ir.InlinePCRanges{instanceA}}
		target := resolveTestLineFile("main.go", []string{"", ""}, []*ir.Subprogram{spA, spB}, withSuperseded)
		require.True(t, target.issue.IsNone(), "%v", target.issue)
		require.Equal(t, otherFile.Name, target.file)
	})
	t.Run("declaring file decides", func(t *testing.T) {
		sp := &ir.Subprogram{InlinePCRanges: []ir.InlinePCRanges{instanceA, instanceB}}
		target := resolveTestLineFile("main.go", []string{mainFile.Name}, []*ir.Subprogram{sp}, data)
		require.True(t, target.issue.IsNone(), "%v", target.issue)
		require.Equal(t, mainFile.Name, target.file)
	})
}

func TestLineProbeCalleeRows(t *testing.T) {
	mainFile := &dwarf.LineFile{Name: "example.com/app/main.go"}
	genFile := &dwarf.LineFile{Name: "example.com/app/gen/main.go"}
	libFile := &dwarf.LineFile{Name: "example.com/lib/main.go"}
	root := ir.PCRange{10, 60}
	ranges := []ir.PCRange{{20, 40}}
	// Code inlined into the method runs at [30, 40).
	calleeRanges := []ir.PCRange{{30, 40}}
	resolve := func(sourceFile, declFile string, callFiles []string, unknown bool, rows []line) (uint64, ir.Issue) {
		sp := &ir.Subprogram{ID: 1, InlinePCRanges: []ir.InlinePCRanges{{Ranges: ranges, RootRanges: []ir.PCRange{root}}}}
		p := &pendingSubprogram{
			id: sp.ID, declFile: declFile, callFiles: callFiles, calleeRanges: calleeRanges, calleeRangesUnknown: unknown,
			probesCfgs: []ir.ProbeDefinition{testLineProbe(sourceFile)},
		}
		data := map[ir.PCRange]lineData{root: {lines: rows}}
		target := resolveLineProbeFiles(
			[]*pendingSubprogram{p}, map[ir.SubprogramID]*ir.Subprogram{sp.ID: sp}, data,
		)["line_probe"]
		if !target.issue.IsNone() {
			return 0, target.issue
		}
		pc, issue, err := pickLineInjectionPC(9, sourceFile, target.file, ranges, []ir.PCRange{root}, data)
		require.NoError(t, err)
		return pc, issue
	}

	for _, tc := range []struct {
		name       string
		sourceFile string
		declFile   string
		callFiles  []string
		unknown    bool
		rows       []line
		want       uint64
		message    string
	}{
		{
			name:       "own file known only from a nested call site",
			sourceFile: "main.go",
			callFiles:  []string{mainFile.Name},
			rows:       []line{{pc: 32, line: 9, isStatement: true, file: libFile}},
			message:    `no code for line 9 of source file "main.go" in the method`,
		},
		{
			name:       "nested call site does not settle two own files at the line",
			sourceFile: "main.go",
			callFiles:  []string{genFile.Name},
			rows: []line{
				{pc: 22, line: 9, isStatement: true, file: mainFile},
				{pc: 26, line: 9, isStatement: true, file: genFile},
			},
			message: `source file "main.go" matches more than one file with code at line 9: ` +
				`["example.com/app/gen/main.go" "example.com/app/main.go"]`,
		},
		{
			name:       "exact callee file without code at the line",
			sourceFile: "/build/example.com/app/main.go",
			declFile:   mainFile.Name,
			rows: []line{
				{pc: 22, line: 9, isStatement: true, file: mainFile},
				{pc: 32, line: 3, isStatement: true, file: &dwarf.LineFile{Name: "/build/example.com/app/main.go"}},
			},
			message: `no code for line 9 of source file "/build/example.com/app/main.go" in the method`,
		},
		{
			name:       "own file before same-named callee file",
			sourceFile: "main.go",
			rows: []line{
				{pc: 22, line: 9, isStatement: true, file: mainFile},
				{pc: 32, line: 9, isStatement: true, file: libFile},
			},
			want: 22,
		},
		{
			name:       "callee file first in pc order",
			sourceFile: "main.go",
			rows: []line{
				{pc: 20, line: 4, isStatement: true, file: mainFile},
				{pc: 30, line: 9, isStatement: true, file: libFile},
				{pc: 35, line: 9, isStatement: true, file: mainFile},
			},
			want: 35,
		},
		{
			name:       "caller line state at the range start",
			sourceFile: "main.go",
			rows: []line{
				{pc: 18, line: 9, isStatement: true, file: &dwarf.LineFile{Name: "example.com/caller/main.go"}},
				{pc: 24, line: 9, isStatement: true, file: mainFile},
			},
			want: 24,
		},
		{
			name:       "caller line state closer to source file than own row",
			sourceFile: "app/main.go",
			rows: []line{
				{pc: 18, line: 9, isStatement: true, file: &dwarf.LineFile{Name: "app/main.go"}},
				{pc: 24, line: 9, isStatement: true, file: mainFile},
			},
			want: 24,
		},
		{
			name:       "caller line state and an own row at another line",
			sourceFile: "pkg/main.go",
			rows: []line{
				{pc: 18, line: 9, isStatement: true, file: &dwarf.LineFile{Name: "pkg/main.go"}},
				{pc: 24, line: 10, isStatement: true, file: &dwarf.LineFile{Name: "example.com/pkg/main.go"}},
			},
			want: 20,
		},
		{
			name:       "caller line state as the only match",
			sourceFile: "caller/main.go",
			rows: []line{
				{pc: 18, line: 9, isStatement: true, file: &dwarf.LineFile{Name: "example.com/caller/main.go"}},
				{pc: 24, line: 9, isStatement: true, file: mainFile},
			},
			want: 20,
		},
		{
			name:       "two own files at the line",
			sourceFile: "main.go",
			rows: []line{
				{pc: 22, line: 9, isStatement: true, file: mainFile},
				{pc: 26, line: 9, isStatement: true, file: genFile},
				{pc: 32, line: 3, isStatement: true, file: libFile},
			},
			message: `source file "main.go" matches more than one file with code at line 9: ` +
				`["example.com/app/gen/main.go" "example.com/app/main.go"]`,
		},
		{
			name:       "own file without code at the line",
			sourceFile: "main.go",
			rows: []line{
				{pc: 22, line: 5, isStatement: true, file: mainFile},
				{pc: 32, line: 9, isStatement: true, file: libFile},
			},
			message: `no code for line 9 of source file "main.go" in the method`,
		},
		{
			name:       "two own files without code at the line",
			sourceFile: "main.go",
			rows: []line{
				{pc: 22, line: 5, isStatement: true, file: mainFile},
				{pc: 26, line: 6, isStatement: true, file: genFile},
				{pc: 32, line: 9, isStatement: true, file: libFile},
			},
			message: `no code for line 9 of source file "main.go" in the method`,
		},
		{
			name:       "source file names the callee file",
			sourceFile: "lib/main.go",
			rows: []line{
				{pc: 22, line: 9, isStatement: true, file: mainFile},
				{pc: 32, line: 9, isStatement: true, file: libFile},
			},
			want: 32,
		},
		{
			name:       "callee ranges unknown",
			sourceFile: "main.go",
			unknown:    true,
			rows: []line{
				{pc: 22, line: 9, isStatement: true, file: mainFile},
				{pc: 32, line: 9, isStatement: true, file: libFile},
			},
			message: `source file "main.go" matches more than one file with code at line 9: ` +
				`["example.com/app/main.go" "example.com/lib/main.go"]`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pc, issue := resolve(tc.sourceFile, tc.declFile, tc.callFiles, tc.unknown, tc.rows)
			if tc.message != "" {
				require.Equal(t, ir.IssueKindInvalidProbeDefinition, issue.Kind)
				require.Equal(t, tc.message, issue.Message)
				return
			}
			require.True(t, issue.IsNone(), "%v", issue)
			require.Equal(t, tc.want, pc)
		})
	}
}

func TestLineProbeSkipsCopyWithoutFileCode(t *testing.T) {
	mainFile := &dwarf.LineFile{Name: "example.com/app/main.go"}
	otherFile := &dwarf.LineFile{Name: "example.com/app/other/main.go"}
	rootA, rootB := ir.PCRange{0x10, 0x40}, ir.PCRange{0x80, 0xc0}
	data := map[ir.PCRange]lineData{
		rootA: {lines: []line{{pc: 0x20, line: 9, isStatement: true, file: otherFile}}},
		rootB: {lines: []line{{pc: 0x90, line: 9, isStatement: true, file: mainFile}}},
	}
	sp := &ir.Subprogram{Name: "main.f", InlinePCRanges: []ir.InlinePCRanges{
		{Ranges: []ir.PCRange{{0x20, 0x30}}, RootRanges: []ir.PCRange{rootA}},
		{Ranges: []ir.PCRange{{0x90, 0xa0}}, RootRanges: []ir.PCRange{rootB}},
	}}
	nop := []byte{0x1f, 0x20, 0x03, 0xd5}
	text := &section{header: &safeelf.SectionHeader{}, data: testSectionData(bytes.Repeat(nop, 0x40))}

	inst, issue, err := newProbeInstance("arm64", testLineProbe("main.go"), sp, mainFile.Name, data, text, false)
	require.NoError(t, err)
	require.True(t, issue.IsNone(), "%v", issue)
	require.Len(t, inst.Events, 1)
	require.Len(t, inst.Events[0].InjectionPoints, 1)
	require.Equal(t, uint64(0x90), inst.Events[0].InjectionPoints[0].PC)

	superseded := maps.Clone(data)
	superseded[rootA] = lineData{lines: []line{
		{pc: 0x18, line: 9, isStatement: true, file: mainFile},
		{pc: 0x1c, line: 4, isStatement: true, file: mainFile},
		{pc: 0x20, line: 9, isStatement: true, file: otherFile},
	}}
	inst, issue, err = newProbeInstance("arm64", testLineProbe("main.go"), sp, mainFile.Name, superseded, text, false)
	require.NoError(t, err)
	require.True(t, issue.IsNone(), "%v", issue)
	require.Len(t, inst.Events[0].InjectionPoints, 1)
	require.Equal(t, uint64(0x90), inst.Events[0].InjectionPoints[0].PC)

	// Rows before the copy's range belong to other code and do not make it skippable.
	before := maps.Clone(data)
	before[rootA] = lineData{lines: []line{
		{pc: 0x14, line: 9, isStatement: true, file: otherFile},
		{pc: 0x18, line: 4, isStatement: true, file: mainFile},
	}}
	_, issue, err = newProbeInstance("arm64", testLineProbe("main.go"), sp, mainFile.Name, before, text, false)
	require.NoError(t, err)
	require.Equal(t, "no suitable injection point found for line: 9", issue.Message)

	sp.InlinePCRanges = sp.InlinePCRanges[:1]
	_, issue, err = newProbeInstance("arm64", testLineProbe("main.go"), sp, mainFile.Name, data, text, false)
	require.NoError(t, err)
	require.Equal(t, `no code for line 9 of source file "main.go" in the method`, issue.Message)

	// Generic shapes are separate subprograms of one probe.
	spA := &ir.Subprogram{ID: 1, Name: "main.f", InlinePCRanges: []ir.InlinePCRanges{
		{Ranges: []ir.PCRange{{0x20, 0x30}}, RootRanges: []ir.PCRange{rootA}},
	}}
	spB := &ir.Subprogram{ID: 2, Name: "main.f", InlinePCRanges: []ir.InlinePCRanges{
		{Ranges: []ir.PCRange{{0x90, 0xa0}}, RootRanges: []ir.PCRange{rootB}},
	}}
	cfg := testLineProbe("main.go")
	pending := []*pendingSubprogram{
		{id: spA.ID, declFile: mainFile.Name, probesCfgs: []ir.ProbeDefinition{cfg}},
		{id: spB.ID, declFile: mainFile.Name, probesCfgs: []ir.ProbeDefinition{cfg}},
	}
	idToSub := map[ir.SubprogramID]*ir.Subprogram{spA.ID: spA, spB.ID: spB}
	probes, _, issues, err := createProbes("arm64", pending, data, idToSub, text, false)
	require.NoError(t, err)
	require.Empty(t, issues)
	require.Len(t, probes, 1)
	require.Len(t, probes[0].Instances, 1)
	require.Equal(t, spB, probes[0].Instances[0].Subprogram)

	probes, _, issues, err = createProbes("arm64", pending[:1], data, idToSub, text, false)
	require.NoError(t, err)
	require.Empty(t, probes)
	require.Len(t, issues, 1)
	require.Equal(t, `no code for line 9 of source file "main.go" in the method`, issues[0].Issue.Message)

	// One issue per probe: a subprogram's own issue is not followed by the deferred one.
	failed := &pendingSubprogram{
		id: 3, probesCfgs: []ir.ProbeDefinition{cfg},
		issue: ir.Issue{Kind: ir.IssueKindInvalidDWARF, Message: "bad ranges"},
	}
	_, _, issues, err = createProbes("arm64", []*pendingSubprogram{failed, pending[0]}, data, idToSub, text, false)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	require.Equal(t, "bad ranges", issues[0].Issue.Message)
}

type testSectionData []byte

func (d testSectionData) Data() []byte { return d }
func (testSectionData) Close() error   { return nil }

// resolveTestLineFile resolves a probe on line 9 of sourceFile across sps,
// the i-th declared in declFiles[i].
func resolveTestLineFile(
	sourceFile string, declFiles []string, sps []*ir.Subprogram, data map[ir.PCRange]lineData,
) lineProbeTarget {
	cfg := testLineProbe(sourceFile)
	idToSub := make(map[ir.SubprogramID]*ir.Subprogram, len(sps))
	pending := make([]*pendingSubprogram, 0, len(sps))
	for i, sp := range sps {
		sp.ID = ir.SubprogramID(i + 1)
		idToSub[sp.ID] = sp
		pending = append(pending, &pendingSubprogram{
			id: sp.ID, declFile: declFiles[i], probesCfgs: []ir.ProbeDefinition{cfg},
		})
	}
	return resolveLineProbeFiles(pending, idToSub, data)["line_probe"]
}

func testLineProbe(sourceFile string) ir.ProbeDefinition {
	return &rcjson.SnapshotProbe{LogProbeCommon: rcjson.LogProbeCommon{ProbeCommon: rcjson.ProbeCommon{
		ID:    "line_probe",
		Type:  rcjson.TypeLogProbe.String(),
		Where: &rcjson.Where{MethodName: "main.f", SourceFile: sourceFile, Lines: []string{"9"}},
	}}}
}

func TestResolveDeclFiles(t *testing.T) {
	mainFile := &dwarf.LineFile{Name: "example.com/app/main.go"}
	otherFile := &dwarf.LineFile{Name: "example.com/app/other/main.go"}
	unit := &dwarf.Entry{Tag: dwarf.TagCompileUnit}
	body := ir.PCRange{100, 200}
	attr := func(a dwarf.Attr, v int64) *dwarf.Entry {
		return &dwarf.Entry{Tag: dwarf.TagSubprogram, Field: []dwarf.Field{{Attr: a, Val: v, Class: dwarf.ClassConstant}}}
	}
	for _, tc := range []struct {
		name  string
		entry *dwarf.Entry
		rows  []line
		want  string
	}{
		{name: "decl_file", entry: attr(dwarf.AttrDeclFile, 1), want: mainFile.Name},
		{name: "decl_file out of range", entry: attr(dwarf.AttrDeclFile, 3)},
		{
			name: "decl_file out of range, body entry at decl_line",
			entry: &dwarf.Entry{Tag: dwarf.TagSubprogram, Field: []dwarf.Field{
				{Attr: dwarf.AttrDeclFile, Val: int64(3), Class: dwarf.ClassConstant},
				{Attr: dwarf.AttrDeclLine, Val: int64(5), Class: dwarf.ClassConstant},
			}},
			rows: []line{{pc: 100, line: 5, file: mainFile}},
			want: mainFile.Name,
		},
		{
			name:  "body entry at decl_line",
			entry: attr(dwarf.AttrDeclLine, 5),
			rows:  []line{{pc: 100, line: 5, file: mainFile}, {pc: 110, line: 9, file: otherFile}},
			want:  mainFile.Name,
		},
		{
			name:  "body entry at another line",
			entry: attr(dwarf.AttrDeclLine, 5),
			rows:  []line{{pc: 100, line: 9, file: otherFile}},
		},
		{
			name:  "no row at body entry",
			entry: attr(dwarf.AttrDeclLine, 5),
			rows:  []line{{pc: 104, line: 5, file: otherFile}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &pendingSubprogram{
				unit:              unit,
				abstractUnit:      unit,
				abstractEntry:     tc.entry,
				outOfLinePCRanges: []ir.PCRange{body},
				probesCfgs:        []ir.ProbeDefinition{testLineProbe("main.go")},
			}
			resolveDeclFiles(
				[]*pendingSubprogram{p},
				map[dwarf.Offset][]*dwarf.LineFile{unit.Offset: {nil, mainFile, otherFile}},
				map[ir.PCRange]lineData{body: {lines: tc.rows}},
			)
			require.Equal(t, tc.want, p.declFile)
		})
	}
}

func TestResolveDeclFilesCallFiles(t *testing.T) {
	mainFile := &dwarf.LineFile{Name: "example.com/app/main.go"}
	instUnit := &dwarf.Entry{Tag: dwarf.TagCompileUnit, Offset: 0x30}
	call := &dwarf.Entry{Tag: dwarf.TagInlinedSubroutine, Field: []dwarf.Field{
		{Attr: dwarf.AttrCallFile, Val: int64(1), Class: dwarf.ClassConstant},
	}}
	p := &pendingSubprogram{
		unit:          instUnit,
		abstractUnit:  instUnit,
		abstractEntry: &dwarf.Entry{Tag: dwarf.TagSubprogram},
		inlined:       []*inlinedSubprogram{{unit: instUnit, nestedInlines: []*dwarf.Entry{call, call}}},
		probesCfgs:    []ir.ProbeDefinition{testLineProbe("main.go")},
	}
	resolveDeclFiles([]*pendingSubprogram{p}, map[dwarf.Offset][]*dwarf.LineFile{instUnit.Offset: {nil, mainFile}}, nil)
	require.Equal(t, []string{mainFile.Name}, p.callFiles)
	require.Empty(t, p.declFile)
}

// A body can sit in another unit than the abstract DIE it shares.
func TestResolveDeclFilesAbstractUnit(t *testing.T) {
	mainFile := &dwarf.LineFile{Name: "example.com/app/main.go"}
	otherFile := &dwarf.LineFile{Name: "example.com/app/other/main.go"}
	abstractUnit := &dwarf.Entry{Tag: dwarf.TagCompileUnit, Offset: 0x10}
	bodyUnit := &dwarf.Entry{Tag: dwarf.TagCompileUnit, Offset: 0x20}
	p := &pendingSubprogram{
		unit:         bodyUnit,
		abstractUnit: abstractUnit,
		abstractEntry: &dwarf.Entry{Tag: dwarf.TagSubprogram, Field: []dwarf.Field{
			{Attr: dwarf.AttrDeclFile, Val: int64(1), Class: dwarf.ClassConstant},
		}},
		probesCfgs: []ir.ProbeDefinition{testLineProbe("main.go")},
	}
	resolveDeclFiles([]*pendingSubprogram{p}, map[dwarf.Offset][]*dwarf.LineFile{
		abstractUnit.Offset: {nil, mainFile},
		bodyUnit.Offset:     {nil, otherFile},
	}, nil)
	require.Equal(t, mainFile.Name, p.declFile)
}

// Discovery and iterConcreteSubprograms read a unit into distinct entries;
// the declaring file must resolve whichever of them a shared range kept.
func TestResolveDeclFilesSharedRange(t *testing.T) {
	cfg := testprogs.MustGetCommonConfigs(t)[0]
	obj, err := object.OpenElfFileWithDwarf(testprogs.MustGetBinary(t, "rc_tester", cfg))
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()
	d := obj.DwarfData()
	r := d.Reader()
	var unit *dwarf.Entry
	var unitRanges [][2]uint64
	for unit == nil {
		e, err := r.Next()
		require.NoError(t, err)
		require.NotNil(t, e)
		if e.Tag == dwarf.TagCompileUnit {
			if unitRanges, err = d.Ranges(e); err == nil && len(unitRanges) > 0 {
				unit = e
			}
		}
		r.SkipChildren()
	}
	r2 := d.Reader()
	r2.Seek(unit.Offset)
	sameUnit, err := r2.Next()
	require.NoError(t, err)
	require.NotSame(t, unit, sameUnit)
	lr, err := d.LineReader(unit)
	require.NoError(t, err)
	files := lr.Files()
	require.Greater(t, len(files), 1)

	shared := ir.PCRange(unitRanges[0])
	lineData, unitFiles, err := collectLineData(d, []lineSearchRange{
		{unit: sameUnit, pcRange: shared},
		{unit: unit, pcRange: shared},
	})
	require.NoError(t, err)
	p := &pendingSubprogram{
		unit: unit,
		subprogramEntry: &dwarf.Entry{Tag: dwarf.TagSubprogram, Field: []dwarf.Field{
			{Attr: dwarf.AttrDeclFile, Val: int64(1), Class: dwarf.ClassConstant},
		}},
		probesCfgs: []ir.ProbeDefinition{testLineProbe("main.go")},
	}
	resolveDeclFiles([]*pendingSubprogram{p}, unitFiles, lineData)
	require.Equal(t, files[1].Name, p.declFile)
}

func TestSourcePathScore(t *testing.T) {
	const want = "example.com/app/main.go"
	ranked := []string{
		want,
		"/" + want,
		"build/" + want,
		"app/main.go",
		"main.go",
	}
	for i := 1; i < len(ranked); i++ {
		require.Greater(t, sourcePathScore(ranked[i-1], want), sourcePathScore(ranked[i], want), ranked[i])
		require.Positive(t, sourcePathScore(ranked[i], want), ranked[i])
	}
	require.Zero(t, sourcePathScore("example.com/lib/main.go", want))
	require.Zero(t, sourcePathScore("pp/main.go", "app/main.go"))
	require.Greater(t, sourcePathScore("/"+want, "/"+want), sourcePathScore(want, "/"+want))
}

func TestLineFileMatches(t *testing.T) {
	const name = "github.com/DataDog/app/pkg/server/main.go"
	for _, tc := range []struct {
		sourceFile string
		want       bool
	}{
		{"main.go", true},
		{"server/main.go", true},
		{"pkg/server/main.go", true},
		{"./pkg/server/main.go", true},
		{"pkg//server/main.go", true},
		{"//pkg/server/main.go", true},
		{name, true},
		{"/home/ci/go/src/" + name, true},
		{"ain.go", false},
		{"handler.go", false},
		{"client/main.go", false},
		{"/main.go", true},
		{`pkg\server\main.go`, true},
		{"../server/main.go", true},
		{"../../client/main.go", false},
		{"..", false},
		{"main.go/", true},
		{"pkg/./server/main.go", true},
		{"pkg/../server/main.go", false},
		{"pkg/server/main.go/../handler.go", false},
		{"pkg/server/main.go/handler.go/..", false},
		{"", false},
		{"/", false},
		{".", false},
	} {
		t.Run(tc.sourceFile, func(t *testing.T) {
			require.Equal(t, tc.want, sourcePathScore(lineFilePath(name), probeSourcePath(tc.sourceFile)) > 0)
		})
	}
}
