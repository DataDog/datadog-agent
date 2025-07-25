// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

const (
	sampleCgroupV2MemoryStat = `anon 3108864
file 2297856
kernel_stack 49152
pagetables 22
percpu 0
sock 0
shmem 2297856
file_mapped 2297856
file_dirty 12
file_writeback 34
anon_thp 0
file_thp 0
shmem_thp 0
inactive_anon 5541888
active_anon 56
inactive_file 78
active_file 90
unevictable 11
slab_reclaimable 0
slab_unreclaimable 0
slab 0
workingset_refault_anon 12
workingset_refault_file 34
workingset_activate_anon 0
workingset_activate_file 0
workingset_restore_anon 0
workingset_restore_file 0
workingset_nodereclaim 0
pgfault 2706
pgmajfault 0
pgrefill 0
pgscan 0
pgsteal 0
pgactivate 0
pgdeactivate 0
pglazyfree 0
pglazyfreed 0
thp_fault_alloc 0
thp_collapse_alloc 0`

	sampleCgroupV2MemoryStatKernel6_8 = `anon 15941632
file 3481600
kernel 929792
kernel_stack 147456
pagetables 348160
sec_pagetables 0
percpu 192
sock 0
vmalloc 0
shmem 0
zswap 0
zswapped 0
file_mapped 2301952
file_dirty 0
file_writeback 0
swapcached 0
anon_thp 0
file_thp 0
shmem_thp 0
inactive_anon 15888384
active_anon 12288
inactive_file 1626112
active_file 1855488
unevictable 0
slab_reclaimable 198720
slab_unreclaimable 199096
slab 397816
workingset_refault_anon 0
workingset_refault_file 850
workingset_activate_anon 0
workingset_activate_file 0
workingset_restore_anon 0
workingset_restore_file 0
workingset_nodereclaim 0
pgscan 0
pgsteal 0
pgscan_kswapd 0
pgscan_direct 0
pgscan_khugepaged 0
pgsteal_kswapd 0
pgsteal_direct 0
pgsteal_khugepaged 0
pgfault 8146
pgmajfault 30
pgrefill 0
pgactivate 453
pgdeactivate 0
pglazyfree 0
pglazyfreed 0
zswpin 0
zswpout 0
zswpwb 0
thp_fault_alloc 0
thp_collapse_alloc 0
thp_swpout 0
thp_swpout_fallback 0`

	sampleCgroupV2MemoryCurrent     = "6193152"
	sampleCgroupV2MemoryMin         = "0"
	sampleCgroupV2MemoryLow         = "0"
	sampleCgroupV2MemoryHigh        = "max"
	sampleCgroupV2MemoryMax         = "max"
	sampleCgroupV2MemoryPeak        = "7000000"
	sampleCgroupV2MemorySwapCurrent = "0"
	sampleCgroupV2MemorySwapHigh    = "max"
	sampleCgroupV2MemorySwapMax     = "max"
	sampleCgroupV2MemoryEvents      = `low 0
high 1
max 2
oom 3
oom_kill 0`
	sampleCgroupV2MemoryPressure = `some avg10=0.00 avg60=0.00 avg300=0.00 total=0
full avg10=0.00 avg60=0.00 avg300=0.00 total=0`
)

func createCgroupV2FakeMemoryFiles(cfs *cgroupMemoryFS, cg *cgroupV2, statContent string) {
	cfs.setCgroupV2File(cg, "memory.stat", statContent)
	cfs.setCgroupV2File(cg, "memory.current", sampleCgroupV2MemoryCurrent)
	cfs.setCgroupV2File(cg, "memory.min", sampleCgroupV2MemoryMin)
	cfs.setCgroupV2File(cg, "memory.low", sampleCgroupV2MemoryLow)
	cfs.setCgroupV2File(cg, "memory.high", sampleCgroupV2MemoryHigh)
	cfs.setCgroupV2File(cg, "memory.max", sampleCgroupV2MemoryMax)
	cfs.setCgroupV2File(cg, "memory.peak", sampleCgroupV2MemoryPeak)
	cfs.setCgroupV2File(cg, "memory.swap.current", sampleCgroupV2MemorySwapCurrent)
	cfs.setCgroupV2File(cg, "memory.swap.high", sampleCgroupV2MemorySwapHigh)
	cfs.setCgroupV2File(cg, "memory.swap.max", sampleCgroupV2MemorySwapMax)
	cfs.setCgroupV2File(cg, "memory.events", sampleCgroupV2MemoryEvents)
	cfs.setCgroupV2File(cg, "memory.pressure", sampleCgroupV2MemoryPressure)
}

func TestCgroupV2MemoryStats(t *testing.T) {
	cfs := newCgroupMemoryFS("/test/fs/cgroup")

	var err error
	stats := &MemoryStats{}

	// Test failure if controller missing (memory is missing)
	tr.reset()
	cgFoo1 := cfs.createCgroupV2("foo1", containerCgroupKubePod(true))
	err = cgFoo1.GetMemoryStats(stats)
	assert.ErrorIs(t, err, &ControllerNotFoundError{Controller: "memory"})

	// Test reading files in memory controller, all files missing
	tr.reset()
	cfs.enableControllers("memory")
	err = cgFoo1.GetMemoryStats(stats)
	assert.NoError(t, err)
	assert.Equal(t, len(tr.errors), 12)
	assert.Empty(t, cmp.Diff(MemoryStats{}, *stats))

	// Test reading files in memory controllers, all files present
	tr.reset()
	createCgroupV2FakeMemoryFiles(cfs, cgFoo1, sampleCgroupV2MemoryStat)
	err = cgFoo1.GetMemoryStats(stats)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []error{}, tr.errors)
	assert.Empty(t, cmp.Diff(MemoryStats{
		UsageTotal:    pointer.Ptr(uint64(6193152)),
		RSS:           pointer.Ptr(uint64(3108864)),
		PageTables:    pointer.Ptr(uint64(22)),
		Shmem:         pointer.Ptr(uint64(2297856)),
		FileMapped:    pointer.Ptr(uint64(2297856)),
		FileDirty:     pointer.Ptr(uint64(12)),
		FileWriteback: pointer.Ptr(uint64(34)),
		Cache:         pointer.Ptr(uint64(2297856)),
		Swap:          pointer.Ptr(uint64(0)),
		RSSHuge:       pointer.Ptr(uint64(0)),
		Pgfault:       pointer.Ptr(uint64(2706)),
		Pgmajfault:    pointer.Ptr(uint64(0)),
		InactiveAnon:  pointer.Ptr(uint64(5541888)),
		ActiveAnon:    pointer.Ptr(uint64(56)),
		InactiveFile:  pointer.Ptr(uint64(78)),
		ActiveFile:    pointer.Ptr(uint64(90)),
		Unevictable:   pointer.Ptr(uint64(11)),
		KernelMemory:  pointer.Ptr(uint64(49152)),
		OOMEvents:     pointer.Ptr(uint64(3)),
		OOMKiilEvents: pointer.Ptr(uint64(0)),
		Peak:          pointer.Ptr(uint64(7000000)),
		RefaultAnon:   pointer.Ptr(uint64(12)),
		RefaultFile:   pointer.Ptr(uint64(34)),
		PSISome: PSIStats{
			Avg10:  pointer.Ptr(0.0),
			Avg60:  pointer.Ptr(0.0),
			Avg300: pointer.Ptr(0.0),
			Total:  pointer.Ptr(uint64(0)),
		},
		PSIFull: PSIStats{
			Avg10:  pointer.Ptr(0.0),
			Avg60:  pointer.Ptr(0.0),
			Avg300: pointer.Ptr(0.0),
			Total:  pointer.Ptr(uint64(0)),
		},
	}, *stats))
}

func TestCgroupV2MemoryStatsKernel6_8(t *testing.T) {
	cfs := newCgroupMemoryFS("/test/fs/cgroup")

	var err error
	stats := &MemoryStats{}
	cgFoo1 := cfs.createCgroupV2("foo1", containerCgroupKubePod(true))

	// Enable memory controller
	cfs.enableControllers("memory")

	createCgroupV2FakeMemoryFiles(cfs, cgFoo1, sampleCgroupV2MemoryStatKernel6_8)
	err = cgFoo1.GetMemoryStats(stats)
	assert.NoError(t, err)
	assert.ElementsMatch(t, []error{}, tr.errors)
	assert.Empty(t, cmp.Diff(MemoryStats{
		UsageTotal:    pointer.Ptr(uint64(6193152)),
		Cache:         pointer.Ptr(uint64(3481600)),
		Swap:          pointer.Ptr(uint64(0)),
		RSS:           pointer.Ptr(uint64(15941632)),
		RSSHuge:       pointer.Ptr(uint64(0)),
		Shmem:         pointer.Ptr(uint64(0)),
		FileMapped:    pointer.Ptr(uint64(2301952)),
		FileDirty:     pointer.Ptr(uint64(0)),
		FileWriteback: pointer.Ptr(uint64(0)),
		Pgfault:       pointer.Ptr(uint64(8146)),
		Pgmajfault:    pointer.Ptr(uint64(30)),
		PageTables:    pointer.Ptr(uint64(348160)),
		InactiveAnon:  pointer.Ptr(uint64(15888384)),
		ActiveAnon:    pointer.Ptr(uint64(12288)),
		InactiveFile:  pointer.Ptr(uint64(1626112)),
		ActiveFile:    pointer.Ptr(uint64(1855488)),
		Unevictable:   pointer.Ptr(uint64(0)),
		RefaultAnon:   pointer.Ptr(uint64(0)),
		RefaultFile:   pointer.Ptr(uint64(850)),
		KernelMemory:  pointer.Ptr(uint64(929792)),
		OOMEvents:     pointer.Ptr(uint64(3)),
		OOMKiilEvents: pointer.Ptr(uint64(0)),
		Peak:          pointer.Ptr(uint64(7000000)),
		PSISome: PSIStats{
			Avg10:  pointer.Ptr(0.0),
			Avg60:  pointer.Ptr(0.0),
			Avg300: pointer.Ptr(0.0),
			Total:  pointer.Ptr(uint64(0)),
		},
		PSIFull: PSIStats{
			Avg10:  pointer.Ptr(0.0),
			Avg60:  pointer.Ptr(0.0),
			Avg300: pointer.Ptr(0.0),
			Total:  pointer.Ptr(uint64(0)),
		},
	}, *stats))
}
