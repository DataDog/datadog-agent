// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package ebpf

import (
	"errors"
	"fmt"
	"io"
	"log"

	bpflib "github.com/iovisor/gobpf/elf"
)

// ErrEBPFNotSupported is returned when eBPF is not enabled/supported on the host
var ErrEBPFNotSupported = errors.New("eBPF is not supported")

// Module represents an eBPF module
type Module struct {
	*bpflib.Module
}

// RegisterPerfMap registers a perf ring buffer
func (m *Module) RegisterPerfMap(perfMap *PerfMapDefinition) (*PerfMap, error) {
	bufferLength := perfMap.BufferLength
	if bufferLength == 0 {
		bufferLength = defaultBufferLength
	}

	eventChannel := make(chan []byte, bufferLength)
	lostChannel := make(chan uint64, defaultLostEventSize)

	pm, err := bpflib.InitPerfMap(m.Module, perfMap.Name, eventChannel, lostChannel)
	if err != nil {
		return nil, fmt.Errorf("error initializing perf map: %s", err)
	}

	log.Printf("Registered perf map %s", perfMap.Name)

	return &PerfMap{
		PerfMap:      pm,
		handler:      perfMap.Handler,
		lostHandler:  perfMap.LostHandler,
		eventChannel: eventChannel,
		lostChannel:  lostChannel,
	}, nil
}

// RegisterTable registers an eBPF map with the specified name
func (m *Module) RegisterTable(name string) (*Table, error) {
	hashMap := m.Map(name)
	if hashMap == nil {
		return nil, fmt.Errorf("failed to find table '%s'", name)
	}

	return &Table{Map: m.Map(name), module: m.Module}, nil
}

// eBPFLogSize is the size of the log buffer given to the verifier
const eBPFLogSize = 1000000

// NewModuleFromReader creates an eBPF from a ReaderAt interface that points to
// the ELF file containing the eBPF bytecode
func NewModuleFromReader(reader io.ReaderAt) (*Module, error) {
	module := bpflib.NewModuleFromReaderWithLogSize(reader, eBPFLogSize)
	if module == nil {
		return nil, ErrEBPFNotSupported
	}

	if err := module.Load(nil); err != nil {
		log.Printf("eBPF verifiers logs: %s", string(module.Log()))
		return nil, err
	}

	/*
		map[string]bpflib.SectionParams{
			"maps/" + name: {
				MapMaxEntries: mapMaxEntries,
			},
		})
	*/

	return &Module{module}, nil
}
