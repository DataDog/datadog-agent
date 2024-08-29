// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ebpf provides utility for setting up and instrumenting the bpf code
// used by dynamic instrumentation
package ebpf

import (
	"errors"
	"fmt"
	"io"
	"os"
	"text/template"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/diagnostics"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
		Name:       "events",
		Type:       ebpf.RingBuf,
		MaxEntries: 1 << 24,
	})
	if err != nil {
		return fmt.Errorf("could not create bpf map for sharing events with userspace: %w", err)
	}
	ditypes.EventsRingbuffer = events
	return nil
}

// SetupHeaders sets up the needed header files for probes in a temporary directory
func SetupHeaders() error {
	var err error
	globalTempDirPath, err = os.MkdirTemp("/tmp", "dd-go-di*")
	if err != nil {
		return err
	}

	tempDir, err := os.MkdirTemp(globalTempDirPath, "run-*")
	if err != nil {
		return fmt.Errorf("couldn't create temp directory: %w", err)
	}

	headersPath, err := loadHeadersToTmpfs(tempDir)
	if err != nil {
		return fmt.Errorf("couldn't load headers to tmpfs: %w", err)
	}

	globalHeadersPath = headersPath
	return nil
}

// AttachBPFUprobe attaches the probe to the specified process
func AttachBPFUprobe(procInfo *ditypes.ProcessInfo, probe *ditypes.Probe) error {
	executable, err := link.OpenExecutable(procInfo.BinaryPath)
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", err.Error())
		return fmt.Errorf("could not open proc executable for attaching bpf probe: %w", err)
	}

	spec, err := ebpf.LoadCollectionSpecFromReader(probe.InstrumentationInfo.BPFObjectFileReader)
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", err.Error())
		return fmt.Errorf("could not create bpf collection for probe %s: %w", probe.ID, err)
	}

	mapReplacements := map[string]*ebpf.Map{}
	if probe.ID != ditypes.ConfigBPFProbeID {
		// config probe is special and should not be on the same ringbuffer
		// as the rest of regular events. Despite having the same "events" name,
		// not using the pinned map means the config program uses a different
		// ringbuffer.
		mapReplacements["events"] = ditypes.EventsRingbuffer
	} else {
		configEvents, err := ebpf.NewMap(&ebpf.MapSpec{
			Type:       ebpf.RingBuf,
			MaxEntries: 1 << 24,
		})
		if err != nil {
			return fmt.Errorf("could not create bpf map for receiving probe configurations: %w", err)
		}
		mapReplacements["events"] = configEvents
	}

	// Load the ebpf object
	opts := ebpf.CollectionOptions{
		Maps: ebpf.MapOptions{
			PinPath: bpffs,
		},
		MapReplacements: mapReplacements,
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
		return fmt.Errorf("could not use bpf map for zero value in bpf object: %w", err)
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
	f := func(in io.Reader, out io.Writer) error {
		fileContents, err := io.ReadAll(in)
		if err != nil {
			return err
		}
		programTemplate, err := template.New("program_template").Parse(string(fileContents))
		if err != nil {
			return err
		}
		err = programTemplate.Execute(out, probe)
		if err != nil {
			return err
		}
		return nil
	}

	cfg := ddebpf.NewConfig()
	opts := runtime.CompileOptions{
		AdditionalFlags:  getCFlags(cfg),
		ModifyCallback:   f,
		UseKernelHeaders: true,
	}
	compiledOutput, err := runtime.Dynamicinstrumentation.CompileWithOptions(cfg, opts)
	if err != nil {
		return err
	}
	probe.InstrumentationInfo.BPFObjectFileReader = compiledOutput
	return nil
}

func getCFlags(config *ddebpf.Config) []string {
	cflags := []string{
		"-g",
		"-Wno-unused-variable",
	}
	if config.BPFDebug {
		cflags = append(cflags, "-DDEBUG=1")
	}
	return cflags
}

const (
	compilationStepTimeout = 60 * time.Second
)
