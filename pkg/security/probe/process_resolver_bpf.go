// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux_bpf

package probe

import (
	"os"
	"syscall"
	"time"

	lib "github.com/DataDog/ebpf"
	"github.com/DataDog/ebpf/manager"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/ebpf/probes"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/process"
)

var snapshotProbeIDs = []manager.ProbeIdentificationPair{
	{
		UID:     probes.SecurityAgentUID,
		Section: "kprobe/security_inode_getattr",
	},
}

// ProcCacheEntry this structure holds the container context that we keep in kernel for each process
type ProcCacheEntry struct {
	FileEvent
	ContainerEvent
	TimestampRaw uint64
	Timestamp    time.Time
}

// Bytes returns the bytes representation of process cache entry
func (pc *ProcCacheEntry) Bytes() []byte {
	b := pc.FileEvent.Bytes()
	b = append(b, pc.ContainerEvent.Bytes()...)
	return b
}

// UnmarshalBinary returns the binary representation of itself
func (pc *ProcCacheEntry) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 45 {
		return 0, ErrNotEnoughData
	}

	read, err := unmarshalBinary(data, &pc.FileEvent, &pc.ContainerEvent)
	if err != nil {
		return 0, err
	}

	pc.TimestampRaw = ebpf.ByteOrder.Uint64(data[read : read+8])

	return read + 8, nil
}

// ProcessResolver resolved process context
type ProcessResolver struct {
	probe            *Probe
	resolvers        *Resolvers
	snapshotProbes   []*manager.Probe
	inodeNumlowerMap *lib.Map
	procCacheMap     *lib.Map
	pidCookieMap     *lib.Map
	entryCache       map[uint32]*ProcessResolverEntry
}

func (p *ProcessResolver) AddEntry(pid uint32, entry *ProcessResolverEntry) {
	p.entryCache[pid] = entry
}

func (p *ProcessResolver) DelEntry(pid uint32) {
	delete(p.entryCache, pid)

	pidb := make([]byte, 4)
	ebpf.ByteOrder.PutUint32(pidb, pid)

	p.pidCookieMap.Delete(pidb)
}

func (p *ProcessResolver) resolve(pid uint32) *ProcessResolverEntry {
	pidb := make([]byte, 4)
	ebpf.ByteOrder.PutUint32(pidb, pid)

	cookieb, err := p.pidCookieMap.LookupBytes(pidb)
	if err != nil {
		return nil
	}

	entryb, err := p.procCacheMap.LookupBytes(cookieb)
	if err != nil {
		return nil
	}

	var procCacheEntry ProcCacheEntry
	if _, err := procCacheEntry.UnmarshalBinary(entryb); err != nil {
		return nil
	}

	pathnameStr := procCacheEntry.FileEvent.ResolveInode(p.resolvers)
	if pathnameStr == dentryPathKeyNotFound {
		return nil
	}

	timestamp := p.resolvers.TimeResolver.ResolveMonotonicTimestamp(procCacheEntry.TimestampRaw)

	entry := &ProcessResolverEntry{
		PathnameStr: pathnameStr,
		Timestamp:   timestamp,
	}
	p.AddEntry(pid, entry)

	return entry
}

func (p *ProcessResolver) Resolve(pid uint32) *ProcessResolverEntry {
	entry, ok := p.entryCache[pid]
	if ok {
		return entry
	}

	// fallback request the map directly, the perf event should be delayed
	return p.resolve(pid)
}

func (p *ProcessResolver) Start() error {
	p.inodeNumlowerMap = p.probe.Map("inode_numlower")
	if p.inodeNumlowerMap == nil {
		return errors.New("map inode_numlower not found")
	}
	p.procCacheMap = p.probe.Map("proc_cache")
	if p.procCacheMap == nil {
		return errors.New("map proc_cache not found")
	}
	p.pidCookieMap = p.probe.Map("pid_cookie")
	if p.pidCookieMap == nil {
		return errors.New("map pid_cookie not found")
	}

	return nil
}

func (p *ProcessResolver) snapshot() error {
	processes, err := process.AllProcesses()
	if err != nil {
		return err
	}

	cacheModified := false

	for _, proc := range processes {
		// If Exe is not set, the process is a short lived process and its /proc entry has already expired, move on.
		if len(proc.Exe) == 0 {
			continue
		}

		// Notify that we modified the cache.
		if p.snapshotProcess(uint32(proc.Pid)) {
			cacheModified = true
		}
	}

	// There is a possible race condition where a process could have started right after we did the call to
	// process.AllProcesses and before we inserted the cache entry of its parent. Call Snapshot again until we
	// do not modify the process cache anymore
	if cacheModified {
		return errors.New("cache modified")
	}

	return nil
}

// snapshotProcess snapshots /proc for the provided pid. This method returns true if it updated the kernel process cache.
func (p *ProcessResolver) snapshotProcess(pid uint32) bool {
	entry := ProcCacheEntry{}
	pidb := make([]byte, 4)
	cookieb := make([]byte, 4)
	inodeb := make([]byte, 8)

	// Check if there already is an entry in the pid <-> cookie cache
	ebpf.ByteOrder.PutUint32(pidb, pid)
	if _, err := p.pidCookieMap.LookupBytes(pidb); err == nil {
		// If there is a cookie, there is an entry in cache, we don't need to do anything else
		return false
	}

	// Populate the mount point cache for the process
	if err := p.resolvers.MountResolver.SyncCache(pid); err != nil {
		if !os.IsNotExist(err) {
			log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't sync mount points", pid))
			return false
		}
	}

	// Retrieve the container ID of the process
	containerID, err := p.resolvers.ContainerResolver.GetContainerID(pid)
	if err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't parse container ID", pid))
		return false
	}
	entry.ContainerEvent.ID = string(containerID)

	procExecPath := utils.ProcExePath(pid)

	// Get process filename and pre-fill the cache
	pathnameStr, err := os.Readlink(procExecPath)
	if err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't readlink binary", pid))
		return false
	}
	p.AddEntry(pid, &ProcessResolverEntry{
		PathnameStr: pathnameStr,
	})

	// Get the inode of the process binary
	fi, err := os.Stat(procExecPath)
	if err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't stat binary", pid))
		return false
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't stat binary", pid))
		return false
	}
	entry.Inode = stat.Ino

	// Fetch the numlower value of the inode
	ebpf.ByteOrder.PutUint64(inodeb, stat.Ino)
	numlowerb, err := p.inodeNumlowerMap.LookupBytes(inodeb)
	if err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't retrieve numlower value", pid))
		return false
	}
	entry.OverlayNumLower = int32(ebpf.ByteOrder.Uint32(numlowerb))

	// Generate a new cookie for this pid
	ebpf.ByteOrder.PutUint32(cookieb, utils.NewCookie())

	// Insert the new cache entry and then the cookie
	if err := p.procCacheMap.Put(cookieb, entry.Bytes()); err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't insert cache entry", pid))
		return false
	}
	if err := p.pidCookieMap.Put(pidb, cookieb); err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't insert cookie", pid))
		return false
	}

	return true
}

// startSnapshotProbes starts the probes required for the snapshot to complete
func (p *ProcessResolver) startSnapshotProbes() error {
	for _, sp := range p.snapshotProbes {
		// enable and start the probe
		sp.Enabled = true
		if err := sp.Init(p.probe.manager); err != nil {
			return errors.Wrapf(err, "couldn't init probe %s", sp.GetIdentificationPair())
		}
		if err := sp.Attach(); err != nil {
			return errors.Wrapf(err, "couldn't start probe %s", sp.GetIdentificationPair())
		}
		log.Debugf("probe %s registered", sp.GetIdentificationPair())
	}
	return nil
}

// stopSnapshotProbes stops the snapshot probes
func (p *ProcessResolver) stopSnapshotProbes() {
	for _, sp := range p.snapshotProbes {
		// Stop snapshot probes
		if err := sp.Stop(); err != nil {
			log.Debugf("couldn't stop probe %s: %v", sp.GetIdentificationPair(), err)
		}
		// the probes selectors mechanism of the manager will re-enable this probe if needed
		sp.Enabled = false
		log.Debugf("probe %s stopped", sp.GetIdentificationPair())
	}
	return
}

func (p *ProcessResolver) Snapshot(containerResolver *ContainerResolver, mountResolver *MountResolver) error {
	// start the snapshot probes
	if err := p.startSnapshotProbes(); err != nil {
		return err
	}

	// Select the inode numlower map to prepare for the snapshot
	p.inodeNumlowerMap = p.probe.Map("inode_numlower")
	if p.inodeNumlowerMap == nil {
		return errors.New("inode_numlower BPF_HASH table doesn't exist")
	}

	// Deregister probes
	defer p.stopSnapshotProbes()

	for retry := 0; retry < 5; retry++ {
		if err := p.snapshot(); err == nil {
			return nil
		}
	}

	return errors.New("unable to snapshot processes")
}

// NewProcessResolver returns a new process resolver
func NewProcessResolver(probe *Probe, resolvers *Resolvers) (*ProcessResolver, error) {
	return &ProcessResolver{
		probe:      probe,
		resolvers:  resolvers,
		entryCache: make(map[uint32]*ProcessResolverEntry),
	}, nil
}
