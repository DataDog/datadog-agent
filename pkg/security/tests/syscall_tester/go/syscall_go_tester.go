// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build syscalltesters
// +build syscalltesters

package main

import (
	"bytes"
	"flag"
	"fmt"

	_ "embed"

	manager "github.com/DataDog/ebpf-manager"
)

var (
	bpfLoad  bool
	bpfClone bool
)

//go:embed ebpf_probe.o
var ebpfProbe []byte

func BPFClone(m *manager.Manager) error {
	if _, err := m.CloneMap("cache", "cache_clone", manager.MapOptions{}); err != nil {
		return fmt.Errorf("couldn't clone 'cache' map: %w", err)
	}
	return nil
}

func BPFLoad() error {
	m := &manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					UID:          "MyVFSOpen",
					EBPFSection:  "kprobe/vfs_open",
					EBPFFuncName: "kprobe_vfs_open",
				},
			},
		},
		Maps: []*manager.Map{
			{
				Name: "cache",
			},
		},
	}
	defer func() {
		_ = m.Stop(manager.CleanAll)
	}()

	if err := m.Init(bytes.NewReader(ebpfProbe)); err != nil {
		return fmt.Errorf("failed to initialize manager: %w", err)
	}

	if bpfClone {
		return BPFClone(m)
	}

	return nil
}

func main() {
	flag.BoolVar(&bpfLoad, "load-bpf", false, "load the eBPF progams")
	flag.BoolVar(&bpfClone, "clone-bpf", false, "clone maps")

	flag.Parse()

	if bpfLoad {
		if err := BPFLoad(); err != nil {
			panic(err)
		}
	}
}
