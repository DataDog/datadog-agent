// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen_test

import (
	"flag"
	"fmt"
	"os"
	"path"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irprinter"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

var rewriteFromEnv = func() bool {
	rewrite, _ := strconv.ParseBool(os.Getenv("REWRITE"))
	return rewrite
}()
var rewrite = flag.Bool("rewrite", rewriteFromEnv, "rewrite the test files")

const snapshotDir = "testdata/snapshot"

func TestSnapshotTesting(t *testing.T) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	progs := testprogs.MustGetPrograms(t)
	sem := dyninsttest.MakeSemaphore()
	for _, prog := range progs {
		t.Run(prog, func(t *testing.T) {
			t.Parallel()
			for _, cfg := range cfgs {
				t.Run(cfg.String(), func(t *testing.T) {
					t.Parallel()
					defer sem.Acquire()()
					runTest(t, cfg, prog)
				})
			}
		})
	}
}

func probeConfigsWithMaxReferenceDepth(
	probesCfgs []ir.ProbeDefinition, limit int,
) []ir.ProbeDefinition {
	for _, cfg := range probesCfgs {
		switch cfg := cfg.(type) {
		case *rcjson.LogProbe:
			if cfg.Capture == nil {
				cfg.Capture = new(rcjson.Capture)
				cfg.Capture.MaxReferenceDepth = limit
			}
		case *rcjson.SnapshotProbe:
			if cfg.Capture == nil {
				cfg.Capture = new(rcjson.Capture)
				cfg.Capture.MaxReferenceDepth = limit
			}
		}
	}
	return probesCfgs
}

func runTest(t *testing.T, cfg testprogs.Config, prog string) {
	binPath := testprogs.MustGetBinary(t, prog, cfg)
	probesCfgs := testprogs.MustGetProbeDefinitions(t, prog)
	obj, err := object.OpenElfFileWithDwarf(binPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, obj.Close()) }()
	irp, err := irgen.GenerateIR(1, obj, probesCfgs)
	require.NoError(t, err)
	require.Empty(t, irp.Issues)

	marshaled, err := irprinter.PrintYAML(irp)
	require.NoError(t, err)

	// Make sure that the IR eventually gets to the same set of types as
	// when we don't have a limit.
	for i := 1; ; i++ {
		probesCfgs := testprogs.MustGetProbeDefinitions(t, prog)
		probesCfgs = probeConfigsWithMaxReferenceDepth(probesCfgs, i)
		irWithLimit, err := irgen.GenerateIR(1, obj, probesCfgs)
		require.NoError(t, err)
		require.Empty(t, irWithLimit.Issues)
		if len(irWithLimit.Types) < len(irp.Types) {
			t.Logf(
				"IR with limit %d has %d types < %d types",
				i, len(irWithLimit.Types), len(irp.Types),
			)
			continue
		}
		typeNames := func(p *ir.Program) []string {
			var names []string
			for _, t := range p.Types {
				names = append(names, fmt.Sprintf("%T:%s", t, t.GetName()))
			}
			slices.Sort(names)
			return names
		}
		require.Equal(t, typeNames(irp), typeNames(irWithLimit))
		break
	}

	outputFile := path.Join(snapshotDir, prog+"."+cfg.String()+".yaml")
	if *rewrite {
		tmpFile, err := os.CreateTemp(snapshotDir, "ir.yaml")
		require.NoError(t, err)
		name := tmpFile.Name()
		defer func() { _ = os.Remove(name) }()
		_, err = tmpFile.Write(marshaled)
		require.NoError(t, err)
		require.NoError(t, tmpFile.Close())
		require.NoError(t, os.Rename(name, outputFile))
	} else {
		expected, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		require.Equal(t, string(expected), string(marshaled))
	}
}
