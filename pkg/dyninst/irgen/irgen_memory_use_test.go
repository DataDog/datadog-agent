// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package irgen_test

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

const internalEnv = "IRGEN_MEMORY_USE_INTERNAL_TEST"

// TestIrgenMemoryUse asserts an upper bound on the retained heap size of the
// irgen package to generate an IR program for our sample binary.
//
// The way it works is to exec a subprocess that runs with go gctrace enabled
// and has garbage collection set to be extremely aggressive. Then, from this,
// we can analyze the peak retained heap size and assert that it doesn't exceed
// a certain threshold (determined empirically).
func TestIrgenMemoryUse(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	tmpDir := t.TempDir()
	stderrPath := filepath.Join(tmpDir, "irgen-memory-use-test.stderr")
	env := append(
		os.Environ(),
		internalEnv+"=true",
		"GOMAXPROCS=1",
		"GODEBUG=gctrace=1",
		"--test.run=TestIrgenMemoryUseInternal",
	)
	stderrFile, err := os.Create(stderrPath)
	require.NoError(t, err)
	cmd := exec.Command(os.Args[0], "--test.run=TestIrgenMemoryUseInternal", "--test.count=1")
	cmd.Env = env
	cmd.Stdout = os.Stdout
	cmd.Stderr = stderrFile
	require.NoError(t, cmd.Run())
	_, err = stderrFile.Seek(0, io.SeekStart)
	require.NoError(t, err)

	scanner := bufio.NewScanner(stderrFile)
	const maxMemLimitMB = uint64(5)
	for scanner.Scan() {
		line := scanner.Text()
		match := gcTraceRegexp.FindStringSubmatch(line)
		if match == nil {
			t.Logf("(stderr) %s", line)
			continue
		}
		liveHeap, err := strconv.ParseUint(match[liveIdx], 10, 64)
		require.NoError(t, err)
		assert.LessOrEqualf(
			t, liveHeap, maxMemLimitMB, "live heap %d MB exceeds %d MB\n%s",
			liveHeap, maxMemLimitMB, line,
		)
	}
	require.NoError(t, scanner.Err())
}

// See https://github.com/golang/go/blob/7056c71d/src/runtime/extern.go#L118-L129
//
//	gc # @#s #%: #+#+# ms clock, #+#/#/#+# ms cpu, #->#-># MB, # MB goal, # MB stacks, #MB globals, # P
var (
	gcTraceRegexp = regexp.MustCompile(
		`^gc (?P<gcNum>\d+) @(?P<time>\S+) (?P<percent>\d+)%: ` +
			`(?P<clock>\S+) ms clock, ` +
			`(?P<cpu>\S+) ms cpu, ` +
			`(?P<start>\d+)->(?P<end>\d+)->(?P<live>\d+) MB, ` +
			`(?P<goal>\d+) MB goal, ` +
			`(?P<stacks>\d+) MB stacks, ` +
			`(?P<globals>\d+) MB globals, ` +
			`(?P<processors>\d+) P`)
	liveIdx = gcTraceRegexp.SubexpIndex("live")
)

func TestIrgenMemoryUseInternal(t *testing.T) {
	if ok, _ := strconv.ParseBool(os.Getenv(internalEnv)); !ok {
		t.Skip("IRGEN_MEMORY_USE_INTERNAL_TEST is not set")
	}
	debug.SetGCPercent(1)
	const prog = "sample"
	cfg := testprogs.MustGetCommonConfigs(t)[0]
	binPath := testprogs.MustGetBinary(t, prog, cfg)
	probes := testprogs.MustGetProbeDefinitions(t, prog)

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

	generated, err := generator.GenerateIR(1, binPath, probes)
	require.NoError(t, err)
	runtime.GC()
	runtime.KeepAlive(generated)
}
