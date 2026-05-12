// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package noisyneighbor

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/noisyneighbor/model"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// pmuEvent describes one hardware/software PMU counter we sample. Each event
// is opened in its own perf-event group (group_fd=-1) rather than as a member
// of a shared group: on heterogeneous PMU hardware (e.g. ARM Neoverse, where
// cache events live on a different physical PMU than CPU-pipeline events) the
// kernel refuses to schedule a group whose members come from different PMUs.
// Per-event opens cost one extra syscall per (event, CPU) at read time but
// work uniformly across architectures.
type pmuEvent struct {
	name   string
	typ    uint32
	config uint64
}

// PMU event indices. The const block is the canonical ordering: pmuEvents
// initialises slots by name (idxCycles: {…}) and ReadAll's struct literal
// reads them back the same way, so reordering this list is a compile error
// rather than a silent metric mislabel.
const (
	idxCycles = iota
	idxInstructions
	idxLLCMisses
	idxBranchMisses
	idxCacheReferences
	idxITLBMisses
	idxCPUMigrations
	numPMUEvents
)

// pmuEvents is the fixed set of counters we attempt to sample per cgroup. An
// event whose perf_event_open returns EINVAL/ENOTSUP on probe is recorded as
// unsupported and skipped for all subsequent cgroup opens.
var pmuEvents = [numPMUEvents]pmuEvent{
	idxCycles:          {name: "cycles", typ: unix.PERF_TYPE_HARDWARE, config: unix.PERF_COUNT_HW_CPU_CYCLES},
	idxInstructions:    {name: "instructions", typ: unix.PERF_TYPE_HARDWARE, config: unix.PERF_COUNT_HW_INSTRUCTIONS},
	idxLLCMisses:       {name: "llc_misses", typ: unix.PERF_TYPE_HARDWARE, config: unix.PERF_COUNT_HW_CACHE_MISSES},
	idxBranchMisses:    {name: "branch_misses", typ: unix.PERF_TYPE_HARDWARE, config: unix.PERF_COUNT_HW_BRANCH_MISSES},
	idxCacheReferences: {name: "cache_references", typ: unix.PERF_TYPE_HARDWARE, config: unix.PERF_COUNT_HW_CACHE_REFERENCES},
	idxITLBMisses: {name: "itlb_misses", typ: unix.PERF_TYPE_HW_CACHE,
		config: uint64(unix.PERF_COUNT_HW_CACHE_ITLB) |
			(uint64(unix.PERF_COUNT_HW_CACHE_OP_READ) << 8) |
			(uint64(unix.PERF_COUNT_HW_CACHE_RESULT_MISS) << 16)},
	idxCPUMigrations: {name: "cpu_migrations", typ: unix.PERF_TYPE_SOFTWARE, config: unix.PERF_COUNT_SW_CPU_MIGRATIONS},
}

// perfReadValue is the layout returned by read(fd) when read_format is
// PERF_FORMAT_TOTAL_TIME_ENABLED | PERF_FORMAT_TOTAL_TIME_RUNNING — exactly
// 24 bytes (one counter + scaling-time pair). We never use PERF_FORMAT_GROUP.
type perfReadValue struct {
	Counter uint64
	Enabled uint64
	Running uint64
}

const perfReadValueSize = int(unsafe.Sizeof(perfReadValue{}))

// cgroupPMUEntry holds the open perf fds for one tracked cgroup and the
// last-read counter values used for delta computation across check intervals.
type cgroupPMUEntry struct {
	cgroupID uint64
	path     string
	cgroupFD int
	// eventFDs[i][cpu] is the fd for event i on CPU cpu, or -1 if the event
	// is unsupported on this host. We pre-size to [numPMUEvents][numCPU] and
	// leave unsupported entries as -1 instead of using a ragged slice.
	eventFDs [numPMUEvents][]int
	last     [numPMUEvents]perfReadValue
}

// cgroupPMUManager owns user-space perf-event-open fds scoped to container
// cgroups via PERF_FLAG_PID_CGROUP. It is created once per Probe lifetime, and
// Refresh()/ReadAll() are called on each /check invocation.
type cgroupPMUManager struct {
	cgroupRoot string
	numCPU     int
	// supported[i] is true when pmuEvents[i] succeeded on the startup probe;
	// false events are skipped on every subsequent open.
	supported [numPMUEvents]bool

	mu      sync.Mutex
	entries map[uint64]*cgroupPMUEntry
}

// newCgroupPMUManager creates a manager rooted at cgroupRoot. cgroupRoot is
// where the host's cgroup v2 hierarchy is visible from inside the running
// process (system-probe sees the host view via /host/proc/1/root/sys/fs/cgroup).
// The manager probes each event against cgroupRoot to learn which counters
// this kernel/PMU combination accepts.
func newCgroupPMUManager(cgroupRoot string) *cgroupPMUManager {
	m := &cgroupPMUManager{
		cgroupRoot: cgroupRoot,
		numCPU:     runtime.NumCPU(),
		entries:    make(map[uint64]*cgroupPMUEntry),
	}
	m.probeSupported()
	return m
}

// probeSupported attempts to open each pmuEvent against the cgroup tree root
// on CPU 0 and records which events the kernel accepts. We don't read these
// probe events — just opening them confirms the (type, config) combination
// is valid and the calling process has the right capability set.
func (m *cgroupPMUManager) probeSupported() {
	cgroupFD, err := unix.Open(m.cgroupRoot, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		log.Warnf("noisy_neighbor: probe open cgroup root %q: %v (PMU events disabled)", m.cgroupRoot, err)
		return
	}
	defer unix.Close(cgroupFD)

	supportedNames := make([]string, 0, numPMUEvents)
	skippedNames := make([]string, 0)
	for i := range pmuEvents {
		fd, err := openPerfEvent(cgroupFD, 0, pmuEvents[i])
		if err != nil {
			m.supported[i] = false
			skippedNames = append(skippedNames, fmt.Sprintf("%s(%v)", pmuEvents[i].name, err))
			continue
		}
		unix.Close(fd)
		m.supported[i] = true
		supportedNames = append(supportedNames, pmuEvents[i].name)
	}
	log.Infof("noisy_neighbor: PMU events supported=%v unsupported=%v", supportedNames, skippedNames)
}

// Refresh walks the cgroup tree looking for container cgroups and opens perf
// events on any new ones. Cgroups that disappeared since the last refresh have
// their fds closed and entries removed.
func (m *cgroupPMUManager) Refresh() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	seen, err := walkContainerCgroups(m.cgroupRoot)
	if err != nil {
		return fmt.Errorf("walking cgroup tree %q: %w", m.cgroupRoot, err)
	}

	for inode, path := range seen {
		if _, ok := m.entries[inode]; ok {
			continue
		}
		entry, err := m.openCgroupEvents(inode, path)
		if err != nil {
			log.Debugf("noisy_neighbor: open perf events for cgroup %s (inode %d): %v", path, inode, err)
			continue
		}
		m.entries[inode] = entry
	}

	for inode, entry := range m.entries {
		if _, ok := seen[inode]; ok {
			continue
		}
		entry.close()
		delete(m.entries, inode)
	}
	return nil
}

// walkContainerCgroups walks cgroupRoot and returns a {inode → path} map for
// every directory whose basename classifies as a container scope. Pure
// filesystem operation — no perf_event_open, no privileged syscalls — so it's
// the natural seam for unit-testing cgroup discovery without root.
func walkContainerCgroups(cgroupRoot string) (map[uint64]string, error) {
	seen := make(map[uint64]string)
	walkErr := filepath.WalkDir(cgroupRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// ENOENT is expected — cgroups vanish under us all the time as
			// containers exit between WalkDir's readdir and stat. Anything
			// else (EACCES on a permission-denied mount, EIO on a wedged
			// fs) is worth surfacing at debug level so operators can
			// diagnose a misconfigured /host/proc/1/root mount without it
			// looking identical to "no containers ran during this window".
			// d == nil happens when WalkDir cannot stat the root itself;
			// callers of walkContainerCgroups depend on that path returning
			// an empty result so the probe doesn't crash at startup if the
			// cgroupRoot mount hasn't materialised yet.
			if !errors.Is(err, fs.ErrNotExist) {
				log.Debugf("noisy_neighbor: walk %s: %v", path, err)
			}
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() || path == cgroupRoot {
			return nil
		}
		switch classifyCgroupName(d.Name()) {
		case cgroupSkip:
			return fs.SkipDir
		case cgroupContainer:
			inode, err := statInode(path)
			if err != nil {
				log.Debugf("noisy_neighbor: stat %s: %v", path, err)
				return nil
			}
			seen[inode] = path
		}
		return nil
	})
	return seen, walkErr
}

// ReadAll snapshots every tracked cgroup's counters, computes a delta against
// the previously stored value, and returns one CgroupPMUStats keyed by cgroup
// inode. The stored "last" value is updated in place so subsequent calls
// return the delta since this call returned.
//
// Each event's value is scaled by its own enabled/running ratio inside this
// function: when an event was multiplexed (running < enabled), the kernel
// only counted for the running fraction, so we extrapolate to the value the
// counter would have shown at 100% allocation. Events on different physical
// PMUs can multiplex independently, so per-event scaling is required for
// correctness — using one shared ratio would systematically misreport events
// that got a different schedule than the reference event.
//
// EnabledNs/RunningNs in the returned struct are the ratio of the first
// supported event, kept only as a coarse "PMU saturation" signal for metric
// consumers — the counter values themselves are already scaled.
func (m *cgroupPMUManager) ReadAll() map[uint64]model.CgroupPMUStats {
	m.mu.Lock()
	defer m.mu.Unlock()

	stats := make(map[uint64]model.CgroupPMUStats, len(m.entries))
	for inode, entry := range m.entries {
		var scaled [numPMUEvents]uint64
		var enabledNs, runningNs uint64
		var primaryRead bool
		for i := range pmuEvents {
			if !m.supported[i] {
				continue
			}
			cur, ok := entry.readEvent(i)
			if !ok {
				continue
			}
			dCounter := cur.Counter - entry.last[i].Counter
			dEnabled := cur.Enabled - entry.last[i].Enabled
			dRunning := cur.Running - entry.last[i].Running
			scaled[i] = scalePMUDelta(dCounter, dEnabled, dRunning)
			if !primaryRead {
				enabledNs = dEnabled
				runningNs = dRunning
				primaryRead = true
			}
			entry.last[i] = cur
		}
		stats[inode] = model.CgroupPMUStats{
			Cycles:          scaled[idxCycles],
			Instructions:    scaled[idxInstructions],
			LLCMisses:       scaled[idxLLCMisses],
			BranchMisses:    scaled[idxBranchMisses],
			CacheReferences: scaled[idxCacheReferences],
			ITLBMisses:      scaled[idxITLBMisses],
			CPUMigrations:   scaled[idxCPUMigrations],
			EnabledNs:       enabledNs,
			RunningNs:       runningNs,
		}
	}
	return stats
}

// scalePMUDelta extrapolates a raw counter delta back to what it would have
// shown at 100% allocation using the standard counter*enabled/running formula.
// When enabled == running (no multiplexing) the result equals the raw delta.
// When running == 0 the event was paused for the entire window so the result
// is 0 — the sample carries no usable information.
func scalePMUDelta(counter, enabled, running uint64) uint64 {
	if running == 0 {
		return 0
	}
	if enabled == running {
		return counter
	}
	// Compute as counter + counter*(enabled-running)/running rather than
	// counter*enabled/running to keep the multiplication operands small in
	// the common case (small multiplexing slice). Reduces — but does not
	// eliminate — u64 overflow risk on very long check intervals.
	missing := enabled - running
	return counter + (counter*missing)/running
}

// Close releases every fd this manager opened.
func (m *cgroupPMUManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for inode, entry := range m.entries {
		entry.close()
		delete(m.entries, inode)
	}
}

// openCgroupEvents opens every supported event on every CPU, scoped to the
// cgroup directory at path. If a single (event, CPU) open fails after the
// startup probe accepted that event, we record the slot as -1 and continue —
// the read path skips negative fds.
//
// Two correctness checks happen here that aren't obvious from a glance:
//
//  1. After opening cgroupFD, we fstat it and bail if the inode no longer
//     matches the one walkContainerCgroups recorded for this path. On busy
//     hosts a container can exit and a new container can be created with the
//     same path between the walk and the open; without this check we'd open
//     perf events against the new cgroup but key the entry under the old
//     inode, silently miscounting until the next refresh.
//
//  2. After all fds are opened, we do an initial readEvent for each supported
//     event and stash the result as entry.last. Without this, the first
//     ReadAll's delta is current_counter - 0, i.e. everything that
//     accumulated between perf_event_open and the first /check — over-reports
//     the first window for any long-lived cgroup tracked from probe init.
func (m *cgroupPMUManager) openCgroupEvents(inode uint64, path string) (*cgroupPMUEntry, error) {
	cgroupFD, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open cgroup dir %q: %w", path, err)
	}

	var st syscall.Stat_t
	if err := syscall.Fstat(cgroupFD, &st); err != nil {
		unix.Close(cgroupFD)
		return nil, fmt.Errorf("fstat cgroup dir %q: %w", path, err)
	}
	if st.Ino != inode {
		// A different cgroup now lives at this path; the inode we walked is
		// gone. Refusing to register this entry leaves it as a candidate
		// for the next refresh, where walkContainerCgroups will pick up the
		// new inode.
		unix.Close(cgroupFD)
		return nil, fmt.Errorf("cgroup inode changed under us: walked=%d open=%d at %q", inode, st.Ino, path)
	}

	entry := &cgroupPMUEntry{
		cgroupID: inode,
		path:     path,
		cgroupFD: cgroupFD,
	}

	anyOpened := false
	for i := range pmuEvents {
		if !m.supported[i] {
			entry.eventFDs[i] = nil
			continue
		}
		fds := make([]int, m.numCPU)
		for cpu := range fds {
			fd, err := openPerfEvent(cgroupFD, cpu, pmuEvents[i])
			if err != nil {
				log.Debugf("noisy_neighbor: open %s on cgroup %s cpu=%d: %v", pmuEvents[i].name, path, cpu, err)
				fds[cpu] = -1
				continue
			}
			fds[cpu] = fd
			anyOpened = true
		}
		entry.eventFDs[i] = fds
	}
	if !anyOpened {
		entry.close()
		return nil, fmt.Errorf("no PMU events opened for %q", path)
	}

	// Prime entry.last with the current counter value so the first ReadAll
	// returns a delta-since-open rather than the full accumulated counter.
	for i := range pmuEvents {
		if !m.supported[i] {
			continue
		}
		if cur, ok := entry.readEvent(i); ok {
			entry.last[i] = cur
		}
	}

	return entry, nil
}

// openPerfEvent opens a single perf event with group_fd=-1 (its own group).
// read_format gets TOTAL_TIME_ENABLED|TOTAL_TIME_RUNNING so each read also
// returns the kernel's scaling-time bookkeeping for the event.
func openPerfEvent(cgroupFD int, cpu int, ev pmuEvent) (int, error) {
	attr := unix.PerfEventAttr{
		Type:        ev.typ,
		Config:      ev.config,
		Size:        uint32(unsafe.Sizeof(unix.PerfEventAttr{})),
		Read_format: unix.PERF_FORMAT_TOTAL_TIME_ENABLED | unix.PERF_FORMAT_TOTAL_TIME_RUNNING,
	}
	return unix.PerfEventOpen(&attr, cgroupFD, cpu, -1,
		unix.PERF_FLAG_PID_CGROUP|unix.PERF_FLAG_FD_CLOEXEC)
}

func (e *cgroupPMUEntry) close() {
	for i := range e.eventFDs {
		for _, fd := range e.eventFDs[i] {
			if fd >= 0 {
				unix.Close(fd)
			}
		}
		e.eventFDs[i] = nil
	}
	if e.cgroupFD >= 0 {
		unix.Close(e.cgroupFD)
		e.cgroupFD = -1
	}
}

// readEvent reads one event across every CPU fd and returns the sum. Returns
// ok=false only if zero CPUs returned a successful read.
func (e *cgroupPMUEntry) readEvent(eventIdx int) (perfReadValue, bool) {
	var sum perfReadValue
	var v perfReadValue
	bufSlice := (*[perfReadValueSize]byte)(unsafe.Pointer(&v))[:]
	any := false
	for _, fd := range e.eventFDs[eventIdx] {
		if fd < 0 {
			continue
		}
		n, err := unix.Read(fd, bufSlice)
		if err != nil || n != perfReadValueSize {
			continue
		}
		sum.Counter += v.Counter
		sum.Enabled += v.Enabled
		sum.Running += v.Running
		any = true
	}
	return sum, any
}

type cgroupKind int

const (
	cgroupOther     cgroupKind = iota // not a container, but recurse into it
	cgroupContainer                   // a container cgroup, register it
	cgroupSkip                        // skip this subtree entirely
)

// classifyCgroupName decides what to do with a cgroup folder by name. Folder
// names matching ContainerRegexp are container scopes; systemd `.mount` aliases
// and conmon monitor cgroups hold no relevant processes so we skip them.
func classifyCgroupName(name string) cgroupKind {
	if cgroups.ContainerRegexp.FindString(name) == "" {
		return cgroupOther
	}
	if strings.HasSuffix(name, ".mount") ||
		strings.HasPrefix(name, "crio-conmon-") ||
		strings.HasPrefix(name, "libpod-conmon-") {
		return cgroupSkip
	}
	return cgroupContainer
}

func statInode(path string) (uint64, error) {
	var st syscall.Stat_t
	if err := syscall.Stat(path, &st); err != nil {
		return 0, err
	}
	return st.Ino, nil
}
