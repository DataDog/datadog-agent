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
	lru "github.com/hashicorp/golang-lru"
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
		Section: "kretprobe/get_task_exe_file",
	},
}

// InodeInfo holds information related to inode from kernel
type InodeInfo struct {
	MountID         uint32
	OverlayNumLower int32
}

// ProcessResolver resolved process context
type ProcessResolver struct {
	probe          *Probe
	resolvers      *Resolvers
	snapshotProbes []*manager.Probe
	inodeInfoMap   *lib.Map
	procCacheMap   *lib.Map
	pidCookieMap   *lib.Map
	entryCache     *lru.Cache
}

// UnmarshalBinary unmarshals a binary representation of itself
func (i *InodeInfo) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}
	i.MountID = ebpf.ByteOrder.Uint32(data)
	i.OverlayNumLower = int32(ebpf.ByteOrder.Uint32(data[4:]))
	return 8, nil
}

// AddEntry add an entry to the local cache
func (p *ProcessResolver) AddEntry(pid uint32, entry ProcessCacheEntry) {
	p.addEntry(pid, &entry)
}

func (p *ProcessResolver) addEntry(pid uint32, entry *ProcessCacheEntry) {
	// resolve now, so that the dentry cache is up to date
	entry.FileEvent.ResolveInode(p.resolvers)
	entry.FileEvent.ResolveContainerPath(p.resolvers)
	entry.ContainerEvent.ResolveContainerID(p.resolvers)

	if entry.Timestamp.IsZero() {
		entry.Timestamp = p.resolvers.TimeResolver.ResolveMonotonicTimestamp(entry.TimestampRaw)
	}

	// check for an existing entry first to inherit ppid
	prevEntry, ok := p.entryCache.Get(pid)
	if ok {
		entry.PPid = prevEntry.(*ProcessCacheEntry).PPid
	}

	p.entryCache.Add(pid, entry)
}

func (p *ProcessResolver) DelEntry(pid uint32) {
	p.entryCache.Remove(pid)
}

func (p *ProcessResolver) resolve(pid uint32) *ProcessCacheEntry {
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

	var entry ProcessCacheEntry
	if _, err := entry.UnmarshalBinary(entryb); err != nil {
		return nil
	}

	p.addEntry(pid, &entry)

	return &entry
}

// Resolve returns the cache entry for the given pid
func (p *ProcessResolver) Resolve(pid uint32) *ProcessCacheEntry {
	entry, exists := p.entryCache.Get(pid)
	if exists {
		return entry.(*ProcessCacheEntry)
	}

	// fallback request the map directly, the perf event may be delayed
	return p.resolve(pid)
}

func (p *ProcessResolver) Get(pid uint32) *ProcessCacheEntry {
	entry, exists := p.entryCache.Get(pid)
	if exists {
		return entry.(*ProcessCacheEntry)
	}
	return nil
}

// Start starts the resolver
func (p *ProcessResolver) Start() error {
	// initializes the list of snapshot probes
	for _, id := range snapshotProbeIDs {
		probe, ok := p.probe.manager.GetProbe(id)
		if !ok {
			return errors.Errorf("couldn't find probe %s", id)
		}
		p.snapshotProbes = append(p.snapshotProbes, probe)
	}

	p.inodeInfoMap = p.probe.Map("inode_info_cache")
	if p.inodeInfoMap == nil {
		return errors.New("map inode_info_cache not found")
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
		if p.snapshotProcess(proc) {
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

func (p *ProcessResolver) retrieveInodeInfo(inode uint64) (*InodeInfo, error) {
	inodeb := make([]byte, 8)

	ebpf.ByteOrder.PutUint64(inodeb, inode)
	data, err := p.inodeInfoMap.LookupBytes(inodeb)
	if err != nil {
		return nil, err
	}

	if data == nil {
		return nil, errors.New("not found")
	}

	var info InodeInfo
	if _, err := info.UnmarshalBinary(data); err != nil {
		return nil, err
	}

	return &info, nil
}

// snapshotProcess snapshots /proc for the provided pid. This method returns true if it updated the kernel process cache.
func (p *ProcessResolver) snapshotProcess(proc *process.FilledProcess) bool {
	pid := uint32(proc.Pid)

	if _, exists := p.entryCache.Get(pid); exists {
		return false
	}

	// create time
	timestamp := time.Unix(0, proc.CreateTime*int64(time.Millisecond))

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

	procExecPath := utils.ProcExePath(pid)

	// Get process filename and pre-fill the cache
	pathnameStr, err := os.Readlink(procExecPath)
	if err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't readlink binary", pid))
		return false
	}

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
	inode := stat.Ino

	info, err := p.retrieveInodeInfo(inode)
	if err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't retrieve inode info", pid))
		return false
	}

	// preset and add the entry to the cache
	entry := &ProcessCacheEntry{
		FileEvent: FileEvent{
			Inode:           inode,
			OverlayNumLower: info.OverlayNumLower,
			MountID:         info.MountID,
			PathnameStr:     pathnameStr,
		},
		ContainerEvent: ContainerEvent{
			ID: string(containerID),
		},
		Timestamp: timestamp,
		Comm:      proc.Name,
		TTYName:   utils.PidTTY(pid),
	}

	log.Tracef("Add process cache entry: %s %s %d/%d", proc.Name, pathnameStr, pid, inode)

	p.addEntry(pid, entry)

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
	p.inodeInfoMap = p.probe.Map("inode_info_cache")
	if p.inodeInfoMap == nil {
		return errors.New("inode_info_cache BPF_HASH table doesn't exist")
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
	cache, err := lru.New(probe.config.PIDCacheSize)
	if err != nil {
		return nil, err
	}

	return &ProcessResolver{
		probe:      probe,
		resolvers:  resolvers,
		entryCache: cache,
	}, nil
}
