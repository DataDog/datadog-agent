// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux_bpf

package noisyneighbor

import (
	"fmt"
	"math/bits"
	"os"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
)

const (
	maxTrackedCgroups = 128
	maxPMUCPUs        = 128
)

type perfEventDefinition struct {
	mask    uint64
	mapName string
	typeID  uint32
	config  uint64
}

var hardwareEvents = []perfEventDefinition{
	{model.EventCycles, "pmu_cycles", unix.PERF_TYPE_HARDWARE, unix.PERF_COUNT_HW_CPU_CYCLES},
	{model.EventInstructions, "pmu_instructions", unix.PERF_TYPE_HARDWARE, unix.PERF_COUNT_HW_INSTRUCTIONS},
	{model.EventCacheMisses, "pmu_cache_misses", unix.PERF_TYPE_HARDWARE, unix.PERF_COUNT_HW_CACHE_MISSES},
	{model.EventCacheReferences, "pmu_cache_references", unix.PERF_TYPE_HARDWARE, unix.PERF_COUNT_HW_CACHE_REFERENCES},
	{model.EventITLBMisses, "pmu_itlb_misses", unix.PERF_TYPE_HW_CACHE, unix.PERF_COUNT_HW_CACHE_ITLB | unix.PERF_COUNT_HW_CACHE_OP_READ<<8 | unix.PERF_COUNT_HW_CACHE_RESULT_MISS<<16},
	{model.EventBranchMisses, "pmu_branch_misses", unix.PERF_TYPE_HARDWARE, unix.PERF_COUNT_HW_BRANCH_MISSES},
}

type perfEventOpener interface {
	Open(event perfEventDefinition, cpu int) (int, error)
	Close(fd int) error
}

type systemPerfEventOpener struct{}

func (systemPerfEventOpener) Open(event perfEventDefinition, cpu int) (int, error) {
	attr := unix.PerfEventAttr{
		Type:        event.typeID,
		Size:        uint32(unsafe.Sizeof(unix.PerfEventAttr{})),
		Config:      event.config,
		Read_format: unix.PERF_FORMAT_TOTAL_TIME_ENABLED | unix.PERF_FORMAT_TOTAL_TIME_RUNNING,
		Bits:        unix.PerfBitExcludeHv,
	}
	return unix.PerfEventOpen(&attr, -1, cpu, -1, unix.PERF_FLAG_FD_CLOEXEC)
}

func (systemPerfEventOpener) Close(fd int) error { return unix.Close(fd) }

type perfEventSet struct {
	opener perfEventOpener
	byMask map[uint64]map[uint]int
}

func openPerfEvents(configuredMask uint64, cpus []uint, opener perfEventOpener) (*perfEventSet, uint64, uint64) {
	set := &perfEventSet{opener: opener, byMask: make(map[uint64]map[uint]int)}
	if len(cpus) > maxPMUCPUs {
		return set, 0, uint64(bits.OnesCount64(configuredMask & model.HardwareEventMask))
	}
	var effectiveMask uint64
	var errors uint64
	for _, event := range hardwareEvents {
		if configuredMask&event.mask == 0 {
			continue
		}
		fds := make(map[uint]int, len(cpus))
		available := true
		for _, cpu := range cpus {
			fd, err := opener.Open(event, int(cpu))
			if err != nil {
				errors++
				available = false
				break
			}
			fds[cpu] = fd
		}
		if !available {
			for _, fd := range fds {
				_ = opener.Close(fd)
			}
			continue
		}
		set.byMask[event.mask] = fds
		effectiveMask |= event.mask
	}
	return set, effectiveMask, errors
}

func (s *perfEventSet) close() {
	if s == nil {
		return
	}
	for _, fds := range s.byMask {
		for _, fd := range fds {
			_ = s.opener.Close(fd)
		}
	}
	s.byMask = make(map[uint64]map[uint]int)
}

func (s *perfEventSet) fdCount() int {
	count := 0
	if s != nil {
		for _, fds := range s.byMask {
			count += len(fds)
		}
	}
	return count
}

func readOnlineCPUs() ([]uint, error) {
	data, err := os.ReadFile("/sys/devices/system/cpu/online")
	if err != nil {
		return nil, err
	}
	return parseCPUList(string(data))
}

func parseCPUList(spec string) ([]uint, error) {
	seen := make(map[uint]struct{})
	for _, item := range strings.Split(strings.TrimSpace(spec), ",") {
		bounds := strings.SplitN(item, "-", 2)
		low, err := strconv.ParseUint(bounds[0], 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid CPU list %q: %w", spec, err)
		}
		high := low
		if len(bounds) == 2 {
			high, err = strconv.ParseUint(bounds[1], 10, 32)
			if err != nil || high < low {
				return nil, fmt.Errorf("invalid CPU list %q", spec)
			}
		}
		for cpu := low; cpu <= high; cpu++ {
			seen[uint(cpu)] = struct{}{}
		}
	}
	cpus := make([]uint, 0, len(seen))
	for cpu := range seen {
		cpus = append(cpus, cpu)
	}
	sort.Slice(cpus, func(i, j int) bool { return cpus[i] < cpus[j] })
	return cpus, nil
}

func scaleCounter(value, enabled, running uint64) (uint64, bool) {
	if running == 0 {
		return 0, false
	}
	if enabled == running {
		return value, true
	}
	hi, lo := bits.Mul64(value, enabled)
	if hi >= running {
		return 0, false
	}
	result, _ := bits.Div64(hi, lo, running)
	return result, true
}
