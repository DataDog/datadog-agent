//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=readonly -no_std_marshalers -build_tags linux -output_filename cgroup_monitor_easyjson.go cgroup_monitor.go

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package syscalls holds syscalls related files
package syscalls

import (
	"encoding/binary"
	"fmt"
	"hash/maphash"
	"slices"
	"sort"
	"time"

	manager "github.com/DataDog/ebpf-manager"
	lib "github.com/cilium/ebpf"
	"github.com/hashicorp/golang-lru/v2/expirable"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/probe/managerhelper"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup"
	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

var (
	defaultSyscallSerializers        []SyscallSerializer
	defaultNetworkSyscallSerializers []SyscallSerializer

	containerExitedCacheSize = 1024

	seed = maphash.MakeSeed()
)

// syscallMonitorKey matches struct syscall_monitor_key_t { u64 cgroup_id; u64 idx; }
// idx is the bucket index (syscall_id / 64) and cgroup_id identifies the cgroup the syscall was issued in.
type syscallMonitorKey struct {
	CGroupID uint64
	Idx      uint64
}

// SyscallSerializer serializes a syscall
// easyjson:json
type SyscallSerializer struct {
	// Name of the syscall
	Name string `json:"name"`
	// ID of the syscall in the host architecture
	ID int `json:"id"`
}

// SyscallsEventSerializer serializes an event to JSON
// easyjson:json
type SyscallsEventSerializer struct {
	*serializers.ContainerContextSerializer `json:"container,omitempty"`
	// List of syscalls captured to generate the event
	Syscalls []SyscallSerializer `json:"syscalls,omitempty"`
}

// ToJSON serializes the current SnapshotEvent object to JSON
func (s *SyscallsEventSerializer) ToJSON() ([]byte, error) {
	return utils.MarshalEasyJSON(s)
}

// appendSyscall adds syscall to the event's syscall list, skipping it if an
// entry with the same ID is already present.
func (s *SyscallsEventSerializer) appendSyscall(syscall model.Syscall) {
	if !slices.ContainsFunc(s.Syscalls, func(serializer SyscallSerializer) bool {
		return serializer.ID == int(syscall)
	}) {
		s.Syscalls = append(s.Syscalls, SyscallSerializer{
			ID:   int(syscall),
			Name: syscall.String(),
		})
	}
}

func (s *SyscallsEventSerializer) appendSyscallSerializers(syscallSerializers ...SyscallSerializer) {
	for _, syscallSerializer := range syscallSerializers {
		if !slices.ContainsFunc(s.Syscalls, func(serializer SyscallSerializer) bool {
			return serializer.ID == syscallSerializer.ID
		}) {
			s.Syscalls = append(s.Syscalls, syscallSerializer)
		}
	}
}

func (s *SyscallsEventSerializer) appendRelatedSyscalls(syscall model.Syscall) {
	switch syscall {
	case model.SysConnect, model.SysAccept:
		s.appendSyscallSerializers(defaultNetworkSyscallSerializers...)
	}
}

// CGroupMonitor dispatches one custom event per cgroup that has observed syscalls
type CGroupMonitor struct {
	syscallMonitor [2]*lib.Map
	bufferSelector *lib.Map
	activeBuffer   uint32
	cgroupResolver *cgroup.Resolver
	numCPU         int
	Period         time.Duration
	lastSent       time.Time
	// relatedEnabled controls whether the default and related syscalls are
	// appended to each dispatched event.
	relatedEnabled bool
	// dedupCache maps cgroup_id -> fingerprint of the last syscall set that
	// was dispatched for that cgroup. If the next set has the same fingerprint,
	// the event is suppressed.
	dedupCache *expirable.LRU[uint64, uint64]

	// exitedContainer keeps the container context of recently deleted cgroups
	// so the next snapshot drain can still resolve syscalls observed before
	// the cgroup went away.
	exitedContainer *expirable.LRU[uint64, model.ContainerContext]
}

// syscallsFingerprint returns an order-independent fingerprint for a syscall
// set. The input slice is sorted in place so callers must not rely on its
// original order.
func syscallsFingerprint(syscalls []model.Syscall) uint64 {
	sort.Slice(syscalls, func(i, j int) bool { return syscalls[i] < syscalls[j] })

	var h maphash.Hash
	h.SetSeed(seed)
	var buf [8]byte
	for _, s := range syscalls {
		binary.LittleEndian.PutUint64(buf[:], uint64(s))
		h.Write(buf[:])
	}
	return h.Sum64()
}

// SendEvents iterates the active syscall monitor map and dispatches one custom event
// per cgroup. Keys are (cgroup_id, bucket_idx) and values are per-CPU u64 bitmasks where
// bit N represents syscall (bucket_idx*64 + N). It is a no-op until Period has elapsed
// since the previous send.
func (d *CGroupMonitor) SendEvents(dispatchFn func(events.EventMarshaler, containerutils.ContainerID)) error {
	now := time.Now()
	if d.Period > 0 && now.Sub(d.lastSent) < d.Period {
		return nil
	}
	d.lastSent = now

	// swap buffers: tell eBPF to write into the other buffer from now on,
	// then drain the buffer it just vacated.
	d.activeBuffer = 1 - d.activeBuffer
	if err := d.bufferSelector.Put(ebpf.BufferSelectorSyscallMonitorKey, d.activeBuffer); err != nil {
		return fmt.Errorf("failed to swap syscall monitor buffer: %w", err)
	}

	buffer := d.syscallMonitor[1-d.activeBuffer]

	// syscallValues accumulates decoded syscall IDs per cgroup.
	syscallValues := make(map[uint64][]model.Syscall)
	cpuValues := make([]uint64, d.numCPU)

	var key syscallMonitorKey
	iterator := buffer.Iterate()
	for iterator.Next(&key, &cpuValues) {
		// OR together all per-CPU bitmasks to get the complete set for this (cgroup, idx).
		var combined uint64
		for _, v := range cpuValues {
			combined |= v
		}
		if combined == 0 {
			continue
		}

		// Decode the 64-bit bitmask into individual syscall IDs.
		for bit := uint64(0); bit < 64; bit++ {
			if combined&(1<<bit) != 0 {
				syscallID := uint64(key.Idx)*64 + bit
				syscallValues[key.CGroupID] = append(syscallValues[key.CGroupID], model.Syscall(syscallID))
				seclog.Tracef("syscall monitor: cgroup %d observed syscall %d", key.CGroupID, syscallID)
			}
		}
	}
	if err := iterator.Err(); err != nil {
		return fmt.Errorf("syscall monitor map iteration failed: %w", err)
	}

	seclog.Debugf("syscall monitor: found %d cgroups with syscalls", len(syscallValues))

	for cgroupID, syscalls := range syscallValues {
		var containerContext model.ContainerContext

		cgce := d.cgroupResolver.GetCacheEntryByInode(cgroupID)
		if cgce != nil {
			containerContext = cgce.GetContainerContext()
		} else {
			containerContext, _ = d.exitedContainer.Get(cgroupID)
			d.exitedContainer.Remove(cgroupID)
		}
		if containerContext.IsNull() {
			continue
		}

		syscallsFingerprint := syscallsFingerprint(syscalls)
		if prev, ok := d.dedupCache.Get(cgroupID); ok && prev == syscallsFingerprint {
			seclog.Tracef("syscall monitor: skipping cgroup %d, syscalls unchanged", cgroupID)
			continue
		}
		d.dedupCache.Add(cgroupID, syscallsFingerprint)

		ev := &SyscallsEventSerializer{
			ContainerContextSerializer: &serializers.ContainerContextSerializer{
				ID:        string(containerContext.ContainerID),
				Source:    containerContext.ContainerSource.String(),
				CreatedAt: utils.NewEasyjsonTimeIfNotZero(time.Unix(0, int64(containerContext.CreatedAt))),
			},
		}

		if d.relatedEnabled {
			ev.appendSyscallSerializers(defaultSyscallSerializers...)
		}

		for _, syscall := range syscalls {
			ev.appendSyscall(syscall)
			if d.relatedEnabled {
				ev.appendRelatedSyscalls(syscall)
			}
		}

		dispatchFn(ev, containerContext.ContainerID)
	}

	return nil
}

// NewEBPFCGroupMonitor returns a new CGroupMonitor
func NewEBPFCGroupMonitor(cfg *config.RuntimeSecurityConfig, mgr *manager.Manager, resolver *cgroup.Resolver) (*CGroupMonitor, error) {
	numCPU, err := utils.NumCPU()
	if err != nil {
		return nil, fmt.Errorf("couldn't fetch the host CPU count: %w", err)
	}

	monitor := &CGroupMonitor{
		cgroupResolver:  resolver,
		numCPU:          numCPU,
		Period:          cfg.SyscallEventsPeriod,
		relatedEnabled:  cfg.SyscallEventsRelatedEnabled,
		dedupCache:      expirable.NewLRU[uint64, uint64](cfg.SyscallsEventDedupCacheSize, nil, cfg.SyscallsEventDedupCacheTTL),
		exitedContainer: expirable.NewLRU[uint64, model.ContainerContext](containerExitedCacheSize, nil, cfg.SyscallsEventDedupCacheTTL*2),
	}

	fbSyscallMonitor, err := managerhelper.Map(mgr, "fb_syscall_monitor")
	if err != nil {
		return nil, err
	}
	monitor.syscallMonitor[0] = fbSyscallMonitor

	bbSyscallMonitor, err := managerhelper.Map(mgr, "bb_syscall_monitor")
	if err != nil {
		return nil, err
	}
	monitor.syscallMonitor[1] = bbSyscallMonitor

	bufferSelector, err := managerhelper.Map(mgr, "buffer_selector")
	if err != nil {
		return nil, err
	}
	monitor.bufferSelector = bufferSelector

	if err := monitor.cgroupResolver.RegisterListener(cgroup.CGroupDeleted, func(cgce *cgroupModel.CacheEntry) {
		containerContext := cgce.GetContainerContext()
		if containerContext.IsNull() {
			return
		}
		monitor.exitedContainer.Add(cgce.GetCGroupInode(), containerContext)
	}); err != nil {
		return nil, err
	}

	return monitor, nil
}

// makeSyscallSerializers converts a list of syscall IDs into their wire
// representation used by SyscallsEventSerializer. The per-architecture
// default and network syscall sets are populated in arch-specific files.
func makeSyscallSerializers(syscalls []model.Syscall) []SyscallSerializer {
	serializers := make([]SyscallSerializer, 0, len(syscalls))
	for _, syscall := range syscalls {
		serializers = append(serializers, SyscallSerializer{
			ID:   int(syscall),
			Name: syscall.String(),
		})
	}
	return serializers
}
