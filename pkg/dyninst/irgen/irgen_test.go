// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package irgen

import (
	"debug/dwarf"
	"encoding/hex"
	"maps"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
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
			message: "no suitable injection point found for line: 9",
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

func TestLineProbeCallFile(t *testing.T) {
	mainFile := &dwarf.LineFile{Name: "example.com/app/main.go"}
	otherFile := &dwarf.LineFile{Name: "example.com/app/other/main.go"}
	root := ir.PCRange{10, 60}
	sp := &ir.Subprogram{ID: 1, InlinePCRanges: []ir.InlinePCRanges{
		{Ranges: []ir.PCRange{{20, 30}}, RootRanges: []ir.PCRange{root}},
	}}
	resolve := func(callFile string, rows []line) lineProbeTarget {
		p := &pendingSubprogram{id: sp.ID, callFile: callFile, probesCfgs: []ir.ProbeDefinition{testLineProbe("main.go")}}
		return resolveLineProbeFiles(
			[]*pendingSubprogram{p}, map[ir.SubprogramID]*ir.Subprogram{sp.ID: sp},
			map[ir.PCRange]lineData{root: {lines: rows}},
		)["line_probe"]
	}
	both := []line{
		{pc: 20, line: 9, isStatement: true, file: otherFile},
		{pc: 25, line: 9, isStatement: true, file: mainFile},
	}

	target := resolve(mainFile.Name, both)
	require.True(t, target.issue.IsNone(), "%v", target.issue)
	require.Equal(t, mainFile.Name, target.file)

	// A call file without code at the line, as under a //line directive,
	// leaves the choice to the rows.
	remapped := "example.com/app/gen/main.go"
	target = resolve(remapped, []line{{pc: 25, line: 9, isStatement: true, file: mainFile}})
	require.True(t, target.issue.IsNone(), "%v", target.issue)
	require.Equal(t, mainFile.Name, target.file)

	target = resolve(remapped, both)
	require.Contains(t, target.issue.Message, "matches more than one file")
}

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
		name     string
		entry    *dwarf.Entry
		rows     []line
		inlined  []*inlinedSubprogram
		want     string
		wantCall string
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
		{
			name:     "call file of a nested inline",
			entry:    attr(dwarf.AttrDeclLine, 5),
			inlined:  []*inlinedSubprogram{{unit: unit}, {unit: unit, callFile: 1, hasCallFile: true}},
			wantCall: mainFile.Name,
		},
		{
			name:    "no nested inline",
			entry:   attr(dwarf.AttrDeclLine, 5),
			inlined: []*inlinedSubprogram{{unit: unit}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &pendingSubprogram{
				unit:              unit,
				abstractEntry:     tc.entry,
				outOfLinePCRanges: []ir.PCRange{body},
				inlined:           tc.inlined,
				probesCfgs:        []ir.ProbeDefinition{testLineProbe("main.go")},
			}
			if tc.inlined != nil {
				p.outOfLinePCRanges = nil
			}
			resolveDeclFiles(
				[]*pendingSubprogram{p},
				map[*dwarf.Entry][]*dwarf.LineFile{unit: {nil, mainFile, otherFile}},
				map[ir.PCRange]lineData{body: {lines: tc.rows}},
			)
			require.Equal(t, tc.want, p.declFile)
			require.Equal(t, tc.wantCall, p.callFile)
		})
	}
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
			file := &dwarf.LineFile{Name: name}
			require.Equal(t, tc.want, lineFileMatches(file, probeSourcePath(tc.sourceFile)))
		})
	}
	require.False(t, lineFileMatches(nil, "main.go"))
}
