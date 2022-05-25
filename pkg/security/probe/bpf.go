// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	internal "github.com/DataDog/btf-internals"
	"github.com/DataDog/btf-internals/sys"
	"github.com/cilium/ebpf"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
)

func getCheckHelperCallInputType(probe *Probe) uint64 {
	input := uint64(1)

	switch {
	case probe.kernelVersion.Code != 0 && probe.kernelVersion.Code >= kernel.Kernel5_13:
		input = uint64(2)
	}

	return input
}

func haveMmapableMaps() error {
	// This checks BPF_F_MMAPABLE, which appeared in 5.5 for array maps.
	m, err := sys.MapCreate(&sys.MapCreateAttr{
		MapType:    sys.MapType(ebpf.Array),
		KeySize:    4,
		ValueSize:  4,
		MaxEntries: 1,
		MapFlags:   unix.BPF_F_MMAPABLE,
	})
	if err != nil {
		return internal.ErrNotSupported
	}
	_ = m.Close()
	return nil
}

func haveRingBuffers() error {
	// This checks ring buffer maps, which appeared in ???.
	m, err := sys.MapCreate(&sys.MapCreateAttr{
		MapType:    sys.MapType(ebpf.RingBuf),
		MaxEntries: 4096 * 16,
	})
	if err != nil {
		return internal.ErrNotSupported
	}
	_ = m.Close()
	return nil
}
