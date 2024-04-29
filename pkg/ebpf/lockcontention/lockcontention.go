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

const (
	bpfObjectFile = "pkg/ebpf/bytecode/build/co-re/lock_contention.o"
)

type bpfPrograms struct {
	KprobeVfsIoctl *ebpf.Program `ebpf:"kprobe__do_vfs_ioctl"`
}

type bpfMaps struct {
	MapFdAddr *ebpf.Map `ebpf:"map_addr_fd"`
}

type bpfObjects struct {
	bpfPrograms
	bpfMaps
}

type targetMap struct {
	fd     int
	id     uint32
	name   string
	mpInfo *ebpf.MapInfo
}

const (
	bpfMapFops      = "bpf_map_fops"
	perCPUOffset    = "__per_cpu_offset"
	cpus            = "num_cpus"
	log2NumOfRanges = "log2_num_of_ranges"
	numOfRanges     = "num_of_ranges"
)

var constants = map[string]interface{}{
	bpfMapFops:   uint64(0),
	perCPUOffset: uint64(0),
	cpus:         uint64(0),
}

func hashMapLockRanges(info *ebpf.MapInfo) uint32 {
	// buckets locks + pcpu freelist locks
	return 2
}

func estimateNumOfLockRanges(tm []targetMap) (uint32, error) {
	var num uint32

	for _, m := range tm {
		switch m.mpInfo.Type {
		case ebpf.Hash:
			num += hashMapLockRanges(m.mpInfo)
		case ebpf.PerCPUHash:
			num += hashMapLockRanges(m.mpInfo)
		default:
		}
	}

	return num, nil
}

func LoadBPFProgram() error {
	var name string
	var err error

	var maps []targetMap

	mapid := ebpf.MapID(0)
	for mapid, err = ebpf.MapGetNextID(mapid); err == nil; mapid, err = ebpf.MapGetNextID(mapid) {
		mp, err := ebpf.NewMapFromID(mapid)
		if err != nil {
			continue
		}

		info, err := mp.Info()
		if err != nil {
			return err
		}

		if name, err = ddebpf.GetMapNameFromMapID(uint32(mapid)); err != nil {
			// this map is not tracked as part of system-probe
			name = info.Name
		}

		maps = append(maps, targetMap{mp.FD(), uint32(mapid), name, info})
	}

	for _, m := range maps {
		fmt.Println(m.name)
	}

	objs := bpfObjects{}
	if err := ddebpf.LoadCOREAsset(bpfObjectFile, func(bc bytecode.AssetReader, managerOptions manager.Options) error {
		collectionSpec, err := ebpf.LoadCollectionSpecFromReader(bc)
		if err != nil {
			return fmt.Errorf("failed to load collection spec: %w", err)
		}

		collectionSpec.Maps["map_addr_fd"].MaxEntries = 1000

		if err := collectionSpec.RewriteConstants(map[string]interface{}{
			"bpf_map_fops":     uint64(0xffffffffa6839880),
			"__per_cpu_offset": uint64(0xffffffffa6e95b40),
			"num_cpus":         uint64(20),
		}); err != nil {
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
