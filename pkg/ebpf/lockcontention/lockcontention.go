// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package lockcontention

import (
	"fmt"
	"syscall"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	manager "github.com/DataDog/ebpf-manager"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
)

type bpfPrograms struct {
	KprobeVfsIoctl *ebpf.Program `ebpf:"kprobe__do_vfs_ioctl"`
}

type bpfMaps struct {
	MapFdAddr *ebpf.Map `ebpf:"map_fd_addr"`
}

type bpfObjects struct {
	bpfPrograms
	bpfMaps
}

func LoadBPFProgram() error {
	objs := bpfObjects{}
	file := "pkg/ebpf/bytecode/build/co-re/lock_contention.o"

	if err := ddebpf.LoadCOREAsset(file, func(bc bytecode.AssetReader, managerOptions manager.Options) error {
		collectionSpec, err := ebpf.LoadCollectionSpecFromReader(bc)
		if err != nil {
			return fmt.Errorf("failed to load collection spec: %w", err)
		}

		collectionSpec.Maps["map_fd_addr"].MaxEntries = 1

		if err := collectionSpec.RewriteConstants(map[string]interface{}{"bpf_map_fops": uint64(0xffffffffa6839880)}); err != nil {
			return fmt.Errorf("failed to write constant: %w", err)
		}

		opts := ebpf.CollectionOptions{
			Programs: ebpf.ProgramOptions{
				LogLevel:    ebpf.LogLevelBranch,
				LogSize:     10 * 1024 * 1024,
				KernelTypes: managerOptions.VerifierOptions.Programs.KernelTypes,
			},
		}

		if err := collectionSpec.LoadAndAssign(&objs, &opts); err != nil {
			return fmt.Errorf("failed to load objects: %w", err)
		}

		return nil
	}); err != nil {
		return err
	}

	kp, err := link.Kprobe("do_vfs_ioctl", objs.KprobeVfsIoctl, nil)
	if err != nil {
		return fmt.Errorf("failed to attack kprobe: %w", err)
	}
	defer kp.Close()

	map_fd := objs.MapFdAddr.FD()
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(map_fd), 0x70c13, uintptr(0)); errno != 0 {
		return err
	}

	return nil
}
