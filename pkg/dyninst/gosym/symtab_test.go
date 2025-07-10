// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package gosym

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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
	mef, err := object.NewMMappingElfFile(binPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, mef.Close()) }()
	obj, err := object.NewElfObject(mef.Elf)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()
	ir, err := irgen.GenerateIR(1, obj, probesCfgs)
	require.NoError(t, err)
	require.Empty(t, ir.Issues)

	moduledata, err := object.ParseModuleData(mef)
	require.NoError(t, err)

	goVersion, err := object.ParseGoVersion(mef)
	require.NoError(t, err)

	goDebugSections, err := moduledata.GoDebugSections(mef)
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
		for _, pc := range pcs {
			locations := symtab.LocatePC(pc)
			require.NotEmpty(t, locations)
			fmt.Fprintf(&out, "pc: 0x%x\n", pc)
			for _, location := range locations {
				// Hide path prefixes that depends on toolchain,repository, and GOPATH locations.
				// Just use the file name, which is the last path component, replaces the leading path with *
				i := strings.LastIndex(location.File, "/")
				fmt.Fprintf(&out, "\t%s@*%s:%d\n", location.Function, location.File[i:], location.Line)
			}
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
