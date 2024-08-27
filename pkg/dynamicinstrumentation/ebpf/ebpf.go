// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ebpf provides utility for setting up and instrumenting the bpf code
// used by dynamic instrumentation
package ebpf

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/diagnostics"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

var (
	bpffs = "/sys/fs/bpf" //TODO: Check via `mount(2)`
)

var (
	globalTempDirPath string
	globalHeadersPath string
)

// SetupEventsMap creates the ringbuffer which all programs will use for sending output
func SetupEventsMap() error {
	var err error
	events, err := ebpf.NewMap(&ebpf.MapSpec{
		Type:       ebpf.RingBuf,
		MaxEntries: 1 << 24,
	})
	if err != nil {
		return fmt.Errorf("could not create bpf map for sharing events with userspace: %w", err)
	}
	ditypes.EventsRingbuffer = events
	return nil
}

// AttachBPFUprobe attaches the probe to the specified process
func AttachBPFUprobe(procInfo *ditypes.ProcessInfo, probe *ditypes.Probe) error {
	executable, err := link.OpenExecutable(procInfo.BinaryPath)
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", err.Error())
		return fmt.Errorf("could not open proc executable for attaching bpf probe: %w", err)
	}

	bpfReader, err := os.Open(probe.InstrumentationInfo.BPFObjectFilePath)
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", err.Error())
		return fmt.Errorf("could not open bpf executable for instrumenting bpf probe: %w", err)
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(bpfReader)
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", err.Error())
		return fmt.Errorf("could not create bpf collection for probe %s: %w", probe.ID, err)
	}

	// Load the ebpf object
	opts := ebpf.CollectionOptions{
		Maps: ebpf.MapOptions{
			PinPath: bpffs,
		},
	}

	bpfObject, err := ebpf.NewCollectionWithOptions(spec, opts)
	if err != nil {
		var ve *ebpf.VerifierError
		if errors.As(err, &ve) {
			log.Infof("Verifier error: %+v\n", ve)
		}
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", err.Error())
		return fmt.Errorf("could not load bpf collection for probe %s: %w", probe.ID, err)
	}

	if probe.ID != ditypes.ConfigBPFProbeID {
		// config probe is special and should not be on the same ringbuffer
		// as the rest of regular events. Despite having the same "events" name,
		// not using the pinned map means the config program uses a different
		// ringbuffer.
		bpfObject.Maps["events"] = ditypes.EventsRingbuffer
	} else {
		configEvents, err := ebpf.NewMap(&ebpf.MapSpec{
			Type:       ebpf.RingBuf,
			MaxEntries: 1 << 24,
		})
		if err != nil {
			return fmt.Errorf("could not create bpf map for receiving probe configurations: %w", err)
		}
		bpfObject.Maps["events"] = configEvents
	}

	procInfo.InstrumentationObjects[probe.ID] = bpfObject

	// Populate map used for zero'ing out regions of memory
	zeroValMap, ok := bpfObject.Maps["zeroval"]
	if !ok {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", "could not find bpf map for zero value")
		return fmt.Errorf("could not find bpf map for zero value in bpf object")
	}

	var zeroSlice = make([]uint8, probe.InstrumentationInfo.InstrumentationOptions.ArgumentsMaxSize)
	var index uint32
	err = zeroValMap.Update(index, zeroSlice, 0)
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", "could not find bpf map for zero value")
		return fmt.Errorf("could not use bpf map for zero value in bpf object")
	}

	// Attach BPF probe to function in executable
	bpfProgram, ok := bpfObject.Programs[probe.GetBPFFuncName()]
	if !ok {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", fmt.Sprintf("couldn't find bpf program for symbol %s", probe.FuncName))
		return fmt.Errorf("could not find bpf program for symbol %s", probe.FuncName)
	}

	link, err := executable.Uprobe(probe.FuncName, bpfProgram, &link.UprobeOptions{
		PID: int(procInfo.PID),
	})
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "UPROBE_FAILURE", err.Error())
		return fmt.Errorf("could not attach bpf program via uprobe: %w", err)
	}

	procInfo.SetUprobeLink(probe.ID, &link)
	diagnostics.Diagnostics.SetStatus(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, ditypes.StatusInstalled)

	return nil
}

// CompileBPFProgram compiles the code for a single probe associated with the process given by procInfo
func CompileBPFProgram(procInfo *ditypes.ProcessInfo, probe *ditypes.Probe) error {
	tempDirPath, err := os.MkdirTemp(globalTempDirPath, "run-*")
	if err != nil {
		return err
	}

	bpfSourceFile, err := os.CreateTemp(tempDirPath, "*.bpf.c")
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "COMPILE_ERROR", err.Error())
		return fmt.Errorf("couldn't create temp file for bpf code of probe %s: %s", probe.ID, err)
	}
	defer bpfSourceFile.Close()

	_, err = bpfSourceFile.WriteString(probe.InstrumentationInfo.BPFSourceCode)
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "COMPILE_ERROR", err.Error())
		return fmt.Errorf("couldn't write to temp file for bpf code of probe %s: %s", probe.ID, err)
	}

	objFilePath := filepath.Join(tempDirPath, fmt.Sprintf("%d-%s-go-di.bpf.o", procInfo.PID, probe.ID))
	cFlags := []string{
		"-O2",
		"-g",
		"--target=bpf",
		fmt.Sprintf("-I%s", globalHeadersPath),
		"-o",
		objFilePath,
	}

	// cfg := ddebpf.NewConfig()
	// opts := kernel.HeaderOptions{
	// 	DownloadEnabled: cfg.EnableKernelHeaderDownload,
	// 	Dirs:            cfg.KernelHeadersDirs,
	// 	DownloadDir:     cfg.KernelHeadersDownloadDir,
	// 	AptConfigDir:    cfg.AptConfigDir,
	// 	YumReposDir:     cfg.YumReposDir,
	// 	ZypperReposDir:  cfg.ZypperReposDir,
	// }
	// kernelHeaders := kernel.GetKernelHeaders(opts, nil)
	// if len(kernelHeaders) == 0 {
	// 	return fmt.Errorf("unable to find kernel headers!!")
	// }

	// err = compiler.CompileToObjectFile(bpfSourceFile.Name(), objFilePath, cFlags, []string{})
	err = clang(cFlags, bpfSourceFile.Name(), withStdout(os.Stdout))
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "COMPILE_ERROR", err.Error())
		return fmt.Errorf("couldn't compile BPF object for probe %s: %s", probe.ID, err)
	}

	probe.InstrumentationInfo.BPFObjectFilePath = objFilePath

	return nil
}

var (
	clangBinPath = getClangPath()
)

const (
	compilationStepTimeout = 60 * time.Second
)

func clang(cflags []string, inputFile string, options ...func(*exec.Cmd)) error {
	var clangErr bytes.Buffer

	clangCtx, clangCancel := context.WithTimeout(context.Background(), compilationStepTimeout)
	defer clangCancel()

	cflags = append(cflags, "-c")
	cflags = append(cflags, inputFile)

	compileToBC := exec.CommandContext(clangCtx, clangBinPath, cflags...)
	for _, opt := range options {
		opt(compileToBC)
	}
	compileToBC.Stderr = &clangErr

	err := compileToBC.Run()
	if err != nil {
		var errMsg string
		if clangCtx.Err() == context.DeadlineExceeded {
			errMsg = "operation timed out"
		} else if len(clangErr.String()) > 0 {
			errMsg = clangErr.String()
		} else {
			errMsg = err.Error()
		}
		return fmt.Errorf("clang: %s", errMsg)
	}

	if len(clangErr.String()) > 0 {
		return fmt.Errorf("%s", clangErr.String())
	}
	return nil
}

func withStdout(out io.Writer) func(*exec.Cmd) {
	return func(c *exec.Cmd) {
		c.Stdout = out
	}
}

func getClangPath() string {
	clangPath := os.Getenv("CLANG_PATH")
	if clangPath == "" {
		clangPath = "/opt/datadog-agent/embedded/bin/clang-bpf"
	}
	return clangPath
}
