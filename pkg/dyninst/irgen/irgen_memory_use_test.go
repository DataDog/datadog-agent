// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen_test

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"runtime/trace"
	"strconv"
	"testing"

	xtrace "golang.org/x/exp/trace"

	"github.com/dustin/go-humanize"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

const internalEnv = "IRGEN_MEMORY_USE_INTERNAL_TEST"
const traceOutputEnv = "IRGEN_MEMORY_USE_TRACE_OUTPUT"

// TestIrgenMemoryUse asserts an upper bound on the memory usage of the
// irgen package to generate an IR program for our sample binary.
//
// The way it works is to exec a subprocess that runs with go tracing enabled
// and has garbage collection set to be extremely aggressive. Then, from this,
// we can analyze the peak memory usage and assert that it doesn't exceed a
// certain threshold (determined empirically).
func TestIrgenMemoryUse(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	tmpDir := t.TempDir()
	traceOutputPath := filepath.Join(tmpDir, "irgen-memory-use-test.trace")
	env := append(
		os.Environ(),
		fmt.Sprintf("%s=true", internalEnv),
		fmt.Sprintf("%s=%s", traceOutputEnv, traceOutputPath),
		"--test.run=TestIrgenMemoryUseInternal",
	)
	cmd := exec.Command(os.Args[0], "--test.run=TestIrgenMemoryUseInternal", "--test.count=1")
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Run())
	require.FileExists(t, traceOutputPath)
	f, err := os.Open(traceOutputPath)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	reader, err := xtrace.NewReader(f)
	require.NoError(t, err)
	var maxMem uint64
	const memoryMetricName = "/memory/classes/heap/objects:bytes"
	for {
		event, err := reader.ReadEvent()
		if err != nil {
			require.ErrorIs(t, err, io.EOF)
			break
		}
		if event.Kind() != xtrace.EventMetric {
			continue
		}
		metric := event.Metric()
		if metric.Name != memoryMetricName {
			continue
		}
		maxMem = max(maxMem, metric.Value.Uint64())
	}
	const maxMemLimit = 9 * 1024 * 1024 // 9 MiB
	require.Less(t, maxMem, uint64(maxMemLimit),
		"%s > %s", humanize.IBytes(maxMem), humanize.IBytes(maxMemLimit))
	t.Logf("maxMem: %s", humanize.IBytes(maxMem))
}

func TestIrgenMemoryUseInternal(t *testing.T) {
	if ok, _ := strconv.ParseBool(os.Getenv(internalEnv)); !ok {
		t.Skip("IRGEN_MEMORY_USE_INTERNAL_TEST is not set")
	}
	debug.SetGCPercent(1)
	const prog = "sample"
	cfg := testprogs.MustGetCommonConfigs(t)[0]
	binPath := testprogs.MustGetBinary(t, prog, cfg)
	probes := testprogs.MustGetProbeDefinitions(t, prog)

	traceOutputPath := os.Getenv(traceOutputEnv)
	require.NotEmpty(t, traceOutputPath)
	traceOutputDir := filepath.Dir(traceOutputPath)
	require.DirExists(t, traceOutputDir)
	traceOutput, err := os.CreateTemp(traceOutputDir, "irgen-memory-use-test-*.trace")
	traceOutputTmpPath := traceOutput.Name()
	require.NoError(t, err)
	defer func() { _ = os.Remove(traceOutputTmpPath) }()
	trace.Start(traceOutput)
	diskCache, err := object.NewDiskCache(object.DiskCacheConfig{
		DirPath:                  t.TempDir(),
		RequiredDiskSpaceBytes:   10 * 1024 * 1024,  // require 10 MiB free
		RequiredDiskSpacePercent: 1.0,               // 1% free space
		MaxTotalBytes:            512 * 1024 * 1024, // 512 MiB max cache size
	})
	require.NoError(t, err)
	irgenOptions := []irgen.Option{
		irgen.WithOnDiskGoTypeIndexFactory(diskCache),
		irgen.WithObjectLoader(diskCache),
	}
	generator := irgen.NewGenerator(irgenOptions...)

	_, err = generator.GenerateIR(1, binPath, probes)
	require.NoError(t, err)
	trace.Stop()
	require.NoError(t, traceOutput.Close())
	require.NoError(t, os.Rename(traceOutputTmpPath, traceOutputPath))
}
