// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gosym

import (
	"bytes"
	"debug/dwarf"
	"flag"
	"fmt"
	"os"
	"path"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dwarf/dwarfutil"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

var rewriteFromEnv = func() bool {
	rewrite, _ := strconv.ParseBool(os.Getenv("REWRITE"))
	return rewrite
}()
var rewrite = flag.Bool("rewrite", rewriteFromEnv, "rewrite the test files")

const snapshotDir = "testdata/snapshot"

var cases = []string{"simple"}

func TestSnapshotTesting(t *testing.T) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	for _, caseName := range cases {
		t.Run(caseName, func(t *testing.T) {
			for _, cfg := range cfgs {
				t.Run(cfg.String(), func(t *testing.T) {
					runTest(t, cfg, caseName)
				})
			}
		})
	}
}

func runTest(
	t *testing.T,
	cfg testprogs.Config,
	caseName string,
) {
	binPath := testprogs.MustGetBinary(t, caseName, cfg)
	probesCfgs := testprogs.MustGetProbeDefinitions(t, caseName)
	obj, err := object.OpenElfFile(binPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()
	ir, err := irgen.GenerateIR(1, obj, probesCfgs)
	require.NoError(t, err)
	require.Empty(t, ir.Issues)

	moduledata, err := object.ParseModuleData(obj.Underlying)
	require.NoError(t, err)

	goVersion, err := object.ReadGoVersion(obj.Underlying)
	require.NoError(t, err)

	goDebugSections, err := moduledata.GoDebugSections(obj.Underlying)
	require.NoError(t, err)
	defer func() { require.NoError(t, goDebugSections.Close()) }()

	symtab, err := ParseGoSymbolTable(
		goDebugSections.PcLnTab.Data,
		goDebugSections.GoFunc.Data,
		moduledata.Text,
		moduledata.EText,
		moduledata.MinPC,
		moduledata.MaxPC,
		goVersion,
	)
	require.NoError(t, err)

	var out bytes.Buffer
	for _, sp := range ir.Subprograms {
		pcs := make([]uint64, 0, len(sp.OutOfLinePCRanges)*2)
		for _, pcr := range sp.OutOfLinePCRanges {
			pcs = append(pcs, pcr[0], (pcr[0]+pcr[1])/2)
		}
		var pcIt PCIterator
		if len(pcs) > 0 {
			f := symtab.PCToFunction(pcs[0])
			require.NotNil(t, f)
			pcIt, err = f.PCIterator(nil /* inlinedPcRanges */)
			require.NoError(t, err)
		}
		for _, pc := range pcs {
			locations := symtab.LocatePC(pc)
			require.NotEmpty(t, locations)
			fmt.Fprintf(&out, "pc: 0x%x\n", pc)
			for _, location := range locations {
				fmt.Fprintf(&out, "\t%s@%s:%d\n", location.Function, location.File, location.Line)
			}

			// Check that funcResolver.PCToLine() agrees with symtab.LocatePC().
			line, ok := pcIt.PCToLine(pc)
			require.True(t, ok)
			require.Equal(t, locations[0].Line, line)
		}
	}

	outputFile := path.Join(snapshotDir, caseName+"."+cfg.String()+".out")
	if *rewrite {
		tmpFile, err := os.CreateTemp(snapshotDir, ".out")
		require.NoError(t, err)
		name := tmpFile.Name()
		defer func() { _ = os.Remove(name) }()
		_, err = tmpFile.Write(out.Bytes())
		require.NoError(t, err)
		require.NoError(t, tmpFile.Close())
		require.NoError(t, os.Rename(name, outputFile))
	} else {
		expected, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		require.Equal(t, string(expected), out.String())
	}
}

func TestFuncPCIterator(t *testing.T) {
	binPath := testprogs.MustGetBinary(t, "simple", testprogs.Config{
		GOARCH:      runtime.GOARCH,
		GOTOOLCHAIN: "go1.24.3",
	})
	obj, err := object.OpenElfFile(binPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()

	moduledata, err := object.ParseModuleData(obj.Underlying)
	require.NoError(t, err)

	goVersion, err := object.ReadGoVersion(obj.Underlying)
	require.NoError(t, err)
	goDebugSections, err := moduledata.GoDebugSections(obj.Underlying)
	require.NoError(t, err)
	defer func() { require.NoError(t, goDebugSections.Close()) }()
	symtab, err := ParseGoSymbolTable(
		goDebugSections.PcLnTab.Data,
		goDebugSections.GoFunc.Data,
		moduledata.Text,
		moduledata.EText,
		moduledata.MinPC,
		moduledata.MaxPC,
		goVersion,
	)
	require.NoError(t, err)

	type testCase struct {
		funcName              string
		expectedLines         []uint32
		expectedInlinedRanges bool
	}
	for _, tc := range []testCase{
		{
			funcName:              "main.funcArg",
			expectedLines:         []uint32{80, 81, 82, 80},
			expectedInlinedRanges: false,
		},
		{
			funcName:              "main.stringArg",
			expectedLines:         []uint32{47, 48, 49, 47},
			expectedInlinedRanges: true,
		},
	} {
		t.Run(tc.funcName, func(t *testing.T) {
			funcEntry, err := findFuncInDwarf(obj.DwarfData(), tc.funcName)
			require.NoError(t, err)
			require.NotNil(t, funcEntry)
			inlinedRanges, err := getInlinedRangesForFunc(funcEntry, obj.DwarfData())
			require.NoError(t, err)
			require.Equal(t, tc.expectedInlinedRanges, len(inlinedRanges) > 0)
			lowpc := funcEntry.Val(dwarf.AttrLowpc).(uint64)
			f := symtab.PCToFunction(lowpc)
			require.NotNil(t, f)

			it, err := f.PCIterator(inlinedRanges)
			require.NoError(t, err)
			// Check the iteration twice, to test Reset().
			for i := 0; i < 2; i++ {
				for _, expectedLine := range tc.expectedLines {
					r, ok := it.Next()
					require.True(t, ok)
					require.Equal(t, int(expectedLine), int(r.Line))
				}
				_, ok := it.Next()
				require.False(t, ok)
				it.Reset()
			}
		})
	}
}

// findFuncInDwarf searches for a function with the given name in the DWARF
// data. If found, the reader is left positioned on the function entry and the
// entry is returned. If not found, returns (nil, nil).
func findFuncInDwarf(data *dwarf.Data, funcName string) (*dwarf.Entry, error) {
	reader := data.Reader()
	for entry, err := reader.Next(); entry != nil; entry, err = reader.Next() {
		if err != nil {
			return nil, err
		}
		if entry.Tag == dwarf.TagSubprogram {
			nameField := entry.AttrField(dwarf.AttrName)
			if nameField == nil {
				continue
			}
			name := nameField.Val.(string)
			if name == funcName {
				return entry, nil
			}
		}
	}
	return nil, nil
}

func getInlinedRangesForFunc(funcEntry *dwarf.Entry, data *dwarf.Data) ([]dwarfutil.PCRange, error) {
	reader := data.Reader()
	reader.Seek(funcEntry.Offset)
	_, err := reader.Next()
	if err != nil {
		return nil, err
	}
	pcRanges, err := dwarfutil.ExploreInlinedPcRangesInSubprogram(reader, data)
	if err != nil {
		return nil, err
	}
	return pcRanges, nil
}
