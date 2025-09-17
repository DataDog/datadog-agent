// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gosym_test

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
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
	obj, err := object.OpenElfFileWithDwarf(binPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()
	iro, err := irgen.GenerateIR(1, obj, probesCfgs)
	require.NoError(t, err)
	require.Empty(t, iro.Issues)

	symtab, err := object.OpenGoSymbolTable(binPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, symtab.Close()) }()

	var out bytes.Buffer
	var inlinedSp *ir.Subprogram
	for _, sp := range iro.Subprograms {
		pcs := make([]uint64, 0, len(sp.OutOfLinePCRanges)*2)
		for _, pcr := range sp.OutOfLinePCRanges {
			pcs = append(pcs, pcr[0], (pcr[0]+pcr[1])/2)
		}
		for _, inlined := range sp.InlinePCRanges {
			inlinedSp = sp
			for _, pcr := range inlined.Ranges {
				pcs = append(pcs, pcr[0], (pcr[0]+pcr[1])/2)
			}
		}
		for _, pc := range pcs {
			locations := symtab.LocatePC(pc)
			require.NotEmpty(t, locations)
			fmt.Fprintf(&out, "LocatePC: 0x%x\n", pc)
			for _, location := range locations {
				fmt.Fprintf(&out, "\t%s@%s:%d\n", location.Function, location.File, location.Line)
			}
		}
	}
	require.NotNil(t, inlinedSp)
	pc := inlinedSp.InlinePCRanges[0].Ranges[0][0]
	fmt.Fprintf(&out, "FunctionLines: 0x%x\n", pc)
	lines, err := symtab.FunctionLines(pc)
	require.NoError(t, err)
	names := make([]string, 0, len(lines))
	for name := range lines {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lines := lines[name]
		fmt.Fprintf(&out, "\t%s\n", name)
		for _, line := range lines.Lines {
			fmt.Fprintf(&out, "\t\t[0x%x, 0x%x) %s@%s:%d\n", line.PCLo, line.PCHi, name, lines.File, line.Line)
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
