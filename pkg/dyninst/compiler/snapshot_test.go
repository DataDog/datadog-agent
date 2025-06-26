// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package compiler

import (
	"bytes"
	"flag"
	"os"
	"path"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler/codegen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler/sm"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
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
	elfFile, err := safeelf.Open(binPath)
	require.NoError(t, err)
	obj, err := object.NewElfObject(elfFile)
	require.NoError(t, err)
	defer func() { require.NoError(t, elfFile.Close()) }()
	ir, err := irgen.GenerateIR(1, obj, probeDefs)
	require.NoError(t, err)

	program, err := sm.GenerateProgram(ir)
	require.NoError(t, err)

	var out bytes.Buffer
	_, err = codegen.GenerateCCode(program, &out)
	require.NoError(t, err)

	outputFile := path.Join(snapshotDir, caseName+"."+cfg.String()+".c")
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
