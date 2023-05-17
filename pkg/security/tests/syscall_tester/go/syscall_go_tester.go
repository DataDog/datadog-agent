// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build syscalltesters

package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/syndtr/gocapability/capability"
)

var (
	bpfLoad            bool
	bpfClone           bool
	capsetProcessCreds bool
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
					EBPFFuncName: "kprobe_vfs_open",
				},
			},
		},
		Maps: []*manager.Map{
			{
				Name: "cache",
			},
			{
				Name: "is_discarded_by_inode_gen",
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

func CapsetTest() error {
	threadCapabilities, err := capability.NewPid2(0)
	if err != nil {
		return err
	}
	if err := threadCapabilities.Load(); err != nil {
		return err
	}

	threadCapabilities.Unset(capability.PERMITTED|capability.EFFECTIVE, capability.CAP_SYS_BOOT)
	threadCapabilities.Unset(capability.EFFECTIVE, capability.CAP_WAKE_ALARM)
	return threadCapabilities.Apply(capability.CAPS)
}

func main() {
	flag.BoolVar(&bpfLoad, "load-bpf", false, "load the eBPF progams")
	flag.BoolVar(&bpfClone, "clone-bpf", false, "clone maps")
	flag.BoolVar(&capsetProcessCreds, "process-credentials-capset", false, "capset test content")

	flag.Parse()

	if bpfLoad {
		if err := BPFLoad(); err != nil {
			panic(err)
		}
	}

	if capsetProcessCreds {
		if err := CapsetTest(); err != nil {
			panic(err)
		}
	}
}
