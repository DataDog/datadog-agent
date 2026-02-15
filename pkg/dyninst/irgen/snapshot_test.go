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

func BenchmarkSnapshotTesting(t *testing.B) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	const prog = "sample"
	t.Run(prog, func(t *testing.B) {
		for _, cfg := range cfgs {
			t.Run(cfg.String(), func(t *testing.B) {
				binPath := testprogs.MustGetBinary(t, prog, cfg)
				probesCfgs := testprogs.MustGetProbeDefinitions(t, prog)
				diskCache, err := object.NewDiskCache(object.DiskCacheConfig{
					DirPath:                  t.TempDir(),
					RequiredDiskSpaceBytes:   10 * 1024 * 1024,  // require 10 MiB free
					RequiredDiskSpacePercent: 1.0,               // 1% free space
					MaxTotalBytes:            512 * 1024 * 1024, // 512 MiB max cache size
				})
				require.NoError(t, err)
				obj, err := diskCache.Load(binPath)
				require.NoError(t, err)
				defer func() { require.NoError(t, obj.Close()) }()

				t.ResetTimer()
				for t.Loop() {
					_, err := irgen.GenerateIR(1, obj, probesCfgs,
						irgen.WithOnDiskGoTypeIndexFactory(diskCache),
						irgen.WithObjectLoader(diskCache),
					)
					require.NoError(t, err)
				}
			})
		}
	})
}

func probeConfigsWithMaxReferenceDepth(
	probesCfgs []ir.ProbeDefinition, limit int,
) []ir.ProbeDefinition {
	for _, cfg := range probesCfgs {
		switch cfg := cfg.(type) {
		case *rcjson.LogProbe:
			if cfg.Capture == nil {
				cfg.Capture = new(rcjson.Capture)
				cfg.Capture.MaxReferenceDepth = &limit
			}
		case *rcjson.SnapshotProbe:
			if cfg.Capture == nil {
				cfg.Capture = new(rcjson.Capture)
				cfg.Capture.MaxReferenceDepth = &limit
			}
		case *rcjson.CaptureExpressionProbe:
			if cfg.Capture == nil {
				cfg.Capture = new(rcjson.Capture)
				cfg.Capture.MaxReferenceDepth = &limit
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

	// Make sure things work with the default limits, but don't actually
	// use the results because they might be huge.
	irWithDefaultLimits, err := irgen.GenerateIR(1, obj, probesCfgs)
	require.NoError(t, err)
	// Use tags to communicate expected issues.
	expectedIssues := make(map[string]string)
	for _, cfg := range probesCfgs {
		if issue, ok := testprogs.GetIssueTag(cfg); ok {
			expectedIssues[cfg.GetID()] = issue
		}
	}
	computeGotIssues := func(p *ir.Program) map[string]string {
		gotIssues := make(map[string]string)
		for _, issue := range p.Issues {
			gotIssues[issue.ProbeDefinition.GetID()] = issue.Issue.Kind.String()
		}
		return gotIssues
	}
	require.Equal(t, expectedIssues, computeGotIssues(irWithDefaultLimits))

	{
		_, err := irprinter.PrintYAML(irWithDefaultLimits)
		require.NoError(t, err)
	}

	// Make sure that the IR eventually gets to the same set of types as
	// when we don't have a limit.
	var irWithLimit1 *ir.Program
	for i := 1; ; i++ {
		probesCfgs := testprogs.MustGetProbeDefinitions(t, prog)
		probesCfgs = probeConfigsWithMaxReferenceDepth(probesCfgs, i)
		irWithLimit, err := irgen.GenerateIR(1, obj, probesCfgs)
		require.NoError(t, err)
		require.Equal(t, expectedIssues, computeGotIssues(irWithLimit))

		if i == 1 {
			irWithLimit1 = irWithLimit
		}
		typeNames := func(p *ir.Program) []string {
			var names []string
			for _, t := range p.Types {
				names = append(names, fmt.Sprintf("%T:%s", t, t.GetName()))
			}
			slices.Sort(names)
			return names
		}
		if slices.Equal(typeNames(irWithDefaultLimits), typeNames(irWithLimit)) {
			t.Logf("types converged with limit %d", i)
			break
		}
		require.Less(t, i, 100, "types did not converge in 100 iterations")
		t.Logf(
			"limit %d has %d types < %d types",
			i, len(irWithLimit.Types), len(irWithDefaultLimits.Types),
		)
	}

	// Use the default probe definitions so it's less noisy.
	for i := range irWithLimit1.Probes {
		irWithLimit1.Probes[i].ProbeDefinition = irWithDefaultLimits.Probes[i].ProbeDefinition
	}
	marshaled, err := irprinter.PrintYAML(irWithLimit1)
	require.NoError(t, err)

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
