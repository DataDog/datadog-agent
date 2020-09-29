// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package ebpf

import (
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/avast/retry-go"
	"github.com/pkg/errors"

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

const (
	// eBPFLogSize is the size of the log buffer given to the verifier (2 * 1024 * 1024)
	eBPFLogSize = 2097152

	// number of retry to avoid fail for permission denied
	maxDetachRetry = 5
)

func detach(kprobe *bpflib.Kprobe) error {
	isKretprobe := strings.HasPrefix(kprobe.Name, "kretprobe/")
	var err error
	if isKretprobe {
		funcName := strings.TrimPrefix(kprobe.Name, "kretprobe/")
		err = disableKprobe("r" + funcName)
	} else {
		funcName := strings.TrimPrefix(kprobe.Name, "kprobe/")
		err = disableKprobe("p" + funcName)
	}
	return err
}

// Close detach all the registered kProbes
func (m *Module) Close() error {
	for kprobe := range m.IterKprobes() {
		if err := kprobe.Detach(); err != nil {
			err := retry.Do(func() error {
				return detach(kprobe)
			}, retry.Attempts(maxDetachRetry), retry.Delay(time.Second))

			if err != nil {
				return err
			}
		}
	}

	return m.Module.Close()
}

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
