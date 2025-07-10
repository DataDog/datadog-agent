// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package dyninsttest provides utilities for dyninst integration testing.
package dyninsttest

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/cilium/ebpf/link"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/compiler"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irprinter"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// MinimumKernelVersion is the minimum kernel version required by the ebpf program.
var MinimumKernelVersion = kernel.VersionCode(5, 17, 0)

// SkipIfKernelNotSupported skips the test if the kernel version is not supported.
func SkipIfKernelNotSupported(t *testing.T) {
	curKernelVersion, err := kernel.HostVersion()
	require.NoError(t, err)
	if curKernelVersion < MinimumKernelVersion {
		t.Skipf("Kernel version %v is not supported", curKernelVersion)
	}
}

// SetupLogging is used to have a consistent logging setup for all tests.
// It is best to call this in TestMain.
func SetupLogging() {
	logLevel := os.Getenv("DD_LOG_LEVEL")
	if logLevel == "" {
		logLevel = "debug"
	}
	const defaultFormat = "%l %Date(15:04:05.000000000) @%File:%Line| %Msg%n"
	var format string
	switch formatFromEnv := os.Getenv("DD_LOG_FORMAT"); formatFromEnv {
	case "":
		format = defaultFormat
	case "json":
		format = `{"time":%Ns,"level":"%Level","msg":"%Msg","path":"%RelFile","func":"%Func","line":%Line}%n`
	case "json-short":
		format = `{"t":%Ns,"l":"%Lev","m":"%Msg"}%n`
	default:
		format = formatFromEnv
	}
	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(
		os.Stderr, log.TraceLvl, format,
	)
	if err != nil {
		panic(fmt.Errorf("failed to create logger: %w", err))
	}
	log.SetupLogger(logger, logLevel)
}

// PrepTmpDir creates a temporary directory and a suitable cleanup function.
func PrepTmpDir(t *testing.T, prefix string) (string, func()) {
	dir, err := os.MkdirTemp(os.TempDir(), prefix)
	require.NoError(t, err)
	t.Logf("using temp dir %s", dir)
	return dir, func() {
		preserve, _ := strconv.ParseBool(os.Getenv("KEEP_TEMP"))
		if preserve || t.Failed() {
			t.Logf("leaving temp dir %s for inspection", dir)
		} else {
			require.NoError(t, os.RemoveAll(dir))
		}
	}
}

// GenerateIr generates an IR program based on a binary and a config files.
func GenerateIr(
	t *testing.T,
	tempDir string,
	binPath string,
	cfgName string,
) (*object.ElfFile, *ir.Program) {
	binary, err := safeelf.Open(binPath)
	require.NoError(t, err)
	defer func() { require.NoError(t, binary.Close()) }()

	probes := testprogs.MustGetProbeDefinitions(t, cfgName)

	obj, err := object.NewElfObject(binary)
	require.NoError(t, err)

	irp, err := irgen.GenerateIR(1, obj, probes)
	require.NoError(t, err)
	require.Empty(t, irp.Issues)

	irDump, err := os.Create(filepath.Join(tempDir, "probe.ir.yaml"))
	require.NoError(t, err)
	defer func() { require.NoError(t, irDump.Close()) }()
	irYaml, err := irprinter.PrintYAML(irp)
	require.NoError(t, err)
	_, err = irDump.Write(irYaml)
	require.NoError(t, err)

	return obj, irp
}

// CompileAndLoadBPF compiles an IR program and loads it into the kernel.
func CompileAndLoadBPF(
	t *testing.T,
	tempDir string,
	irp *ir.Program,
) (*loader.Program, func()) {
	codeDump, err := os.Create(filepath.Join(tempDir, "probe.sm.txt"))
	require.NoError(t, err)
	defer func() { require.NoError(t, codeDump.Close()) }()

	smProgram, err := compiler.GenerateProgram(irp)
	require.NoError(t, err)

	loader, err := loader.NewLoader()
	require.NoError(t, err)
	program, err := loader.Load(smProgram)
	require.NoError(t, err)

	return program, func() {
		program.Close()
	}
}

// StartProcess starts a process and returns a write closer for the stdin.
func StartProcess(ctx context.Context, t *testing.T, tempDir string, binPath string, args ...string) (*exec.Cmd, io.WriteCloser) {
	proc := exec.CommandContext(ctx, binPath, args...)
	sampleStdin, err := proc.StdinPipe()
	require.NoError(t, err)
	proc.Stdout, err = os.Create(filepath.Join(tempDir, "sample.out"))
	require.NoError(t, err)
	proc.Stderr, err = os.Create(filepath.Join(tempDir, "sample.err"))
	require.NoError(t, err)
	err = proc.Start()
	require.NoError(t, err)

	require.NoError(t, err)
	return proc, sampleStdin
}

// AttachBPFProbes attaches the BPF program to the running process.
func AttachBPFProbes(
	t *testing.T,
	binPath string,
	obj *object.ElfFile,
	pid int,
	program *loader.Program,
) func() {
	sampleLink, err := link.OpenExecutable(binPath)
	require.NoError(t, err)
	textSection, err := object.FindTextSectionHeader(obj.File)
	require.NoError(t, err)

	var allAttached []link.Link
	for _, attachpoint := range program.Attachpoints {
		// Despite the name, Uprobe expects an offset in the object file, and not the virtual address.
		addr := attachpoint.PC - textSection.Addr + textSection.Offset
		attached, err := sampleLink.Uprobe(
			"",
			program.BpfProgram,
			&link.UprobeOptions{
				PID:     pid,
				Address: addr,
				Offset:  0,
				Cookie:  attachpoint.Cookie,
			},
		)
		require.NoError(t, err)
		allAttached = append(allAttached, attached)
	}
	return func() {
		for _, a := range allAttached {
			require.NoError(t, a.Close())
		}
	}
}
