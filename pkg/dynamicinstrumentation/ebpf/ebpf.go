// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package ebpf provides utility for setting up and instrumenting the bpf code
// used by dynamic instrumentation
package ebpf

import (
	"errors"
	"fmt"
	"io"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"

	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/diagnostics"
	"github.com/DataDog/datadog-agent/pkg/dynamicinstrumentation/ditypes"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	ebpfruntime "github.com/DataDog/datadog-agent/pkg/ebpf/bytecode/runtime"
	template "github.com/DataDog/datadog-agent/pkg/template/text"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

	numCPUs, err := kernel.PossibleCPUs()
	if err != nil {
		numCPUs = 96
		log.Error("unable to detect number of CPUs. assuming 96 cores")
	}
	outerMapSpec := spec.Maps["param_stacks"]
	outerMapSpec.MaxEntries = uint32(numCPUs)

	inner := &ebpf.MapSpec{
		Type:       ebpf.Stack,
		MaxEntries: 2048,
		ValueSize:  8,
	}

	for i := range outerMapSpec.MaxEntries {
		innerMap, err := ebpf.NewMap(inner)
		if err != nil {
			return fmt.Errorf("could not create bpf map for reading memory content: %w", err)
		}
		outerMapSpec.Contents = append(outerMapSpec.Contents,
			ebpf.MapKV{
				Key:   uint32(i),
				Value: innerMap,
			},
		)
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

	if procInfo.InstrumentationObjects == nil {
		procInfo.InstrumentationObjects = ditypes.NewInstrumentationObjectsMap()
	}
	procInfo.InstrumentationObjects.Set(probe.ID, bpfObject)

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
	bpfProgram, ok := bpfObject.Programs[ditypes.GetBPFFuncName(probe)]
	if !ok {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "ATTACH_ERROR", fmt.Sprintf("couldn't find bpf program for symbol %s", probe.FuncName))
		return fmt.Errorf("could not find bpf program for symbol %s", probe.FuncName)
	}

	manager.TraceFSLock.Lock()
	link, err := executable.Uprobe(probe.FuncName, bpfProgram, &link.UprobeOptions{
		PID: int(procInfo.PID),
	})
	manager.TraceFSLock.Unlock()
	if err != nil {
		diagnostics.Diagnostics.SetError(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, "UPROBE_FAILURE", fmt.Sprintf("%s: %s", probe.FuncName, err.Error()))
		return fmt.Errorf("could not attach bpf program for %s via uprobe: %w", probe.FuncName, err)
	}

	procInfo.SetUprobeLink(probe.ID, &link)
	diagnostics.Diagnostics.SetStatus(procInfo.ServiceName, procInfo.RuntimeID, probe.ID, ditypes.StatusInstalled)

	return nil
}

// CompileBPFProgram compiles the code for a single probe
func CompileBPFProgram(probe *ditypes.Probe) error {
	f := func(in io.Reader, out io.Writer) error {
		fileContents, err := io.ReadAll(in)
		if err != nil {
			return err
		}
		programTemplate, err := template.New("program_template").Funcs(template.FuncMap{
			"GetBPFFuncName": ditypes.GetBPFFuncName,
		}).Parse(string(fileContents))
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
	opts := ebpfruntime.CompileOptions{
		AdditionalFlags:  getCFlags(cfg),
		ModifyCallback:   f,
		UseKernelHeaders: true,
	}
	compiledOutput, err := ebpfruntime.Dynamicinstrumentation.CompileWithOptions(cfg, opts)
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
