// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

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
	probeDefs := testprogs.MustGetProbeDefinitions(t, caseName)
	obj, err := object.OpenElfFile(binPath)
	require.NoError(t, err)
	defer func() { _ = obj.Close() }()
	ir, err := irgen.GenerateIR(1, obj, probeDefs)
	require.NoError(t, err)
	require.Empty(t, ir.Issues)

	program, err := GenerateProgram(ir)
	require.NoError(t, err)

	var out bytes.Buffer
	out.WriteString("// Stack machine code\n")
	metadata, err := GenerateCode(program, &DebugSerializer{Out: &out})
	require.NoError(t, err)

	sort.Slice(program.Types, func(i, j int) bool {
		return program.Types[i].GetID() < program.Types[j].GetID()
	})
	out.WriteString("// Types\n")
	for _, t := range program.Types {
		out.WriteString(fmt.Sprintf("ID: %d Len: %d Enqueue: %d\n",
			t.GetID(), t.GetByteSize(), metadata.FunctionLoc[ProcessType{Type: t}]))
	}

	outputFile := path.Join(snapshotDir, caseName+"."+cfg.String()+".sm.txt")
	if *rewrite {
		tmpFile, err := os.CreateTemp(snapshotDir, "sm.c")
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
