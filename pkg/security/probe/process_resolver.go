// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"context"
	"fmt"
	"os"
	"sync"
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
		Section: "kretprobe/get_task_exe_file",
	},
}

// InodeInfo holds information related to inode from kernel
type InodeInfo struct {
	MountID         uint32
	OverlayNumLower int32
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

// ProcessResolver resolved process context
type ProcessResolver struct {
	sync.RWMutex
	probe          *Probe
	resolvers      *Resolvers
	snapshotProbes []*manager.Probe
	inodeInfoMap   *lib.Map
	procCacheMap   *lib.Map
	pidCookieMap   *lib.Map

	entryCache map[uint32]*ProcessCacheEntry
}

// GetProbes returns the probes required by the snapshot
func (p *ProcessResolver) GetProbes() []*manager.Probe {
	return p.snapshotProbes
}

// AddEntry adds an entry to the local cache and returns the newly created entry
func (p *ProcessResolver) AddEntry(pid uint32, entry *ProcessCacheEntry) *ProcessCacheEntry {
	p.Lock()
	defer p.Unlock()
	return p.insertEntry(pid, entry)
}

// DumpCache prints the process cache to the console
func (p *ProcessResolver) DumpCache() {
	fmt.Println("Dumping process cache ...")
	for _, entry := range p.entryCache {
		fmt.Printf("%s\n", entry)
	}
}

// enrichEventFromProc uses /proc to enrich a ProcessCacheEntry with additional metadata
func (p *ProcessResolver) enrichEventFromProc(entry *ProcessCacheEntry, proc *process.FilledProcess) error {
	pid := uint32(proc.Pid)

	// Get process filename and pre-fill the cache
	procExecPath := utils.ProcExePath(pid)
	pathnameStr, err := os.Readlink(procExecPath)
	if err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't readlink binary", pid))
		return err
	}
	if pathnameStr == "/ (deleted)" {
		log.Debugf("snapshot failed for %d: binary was deleted", pid)
		return errors.New("snapshot failed")
	}

	// Get the inode of the process binary
	fi, err := os.Stat(procExecPath)
	if err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't stat binary", pid))
		return err
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		log.Debugf("snapshot failed for %d: couldn't stat binary", pid)
		return errors.New("snapshot failed")
	}
	inode := stat.Ino

	info, err := p.retrieveInodeInfo(inode)
	if err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't retrieve inode info", pid))
		return err
	}

	// Retrieve the container ID of the process from /proc
	containerID, err := p.resolvers.ContainerResolver.GetContainerID(pid)
	if err != nil {
		log.Debug(errors.Wrapf(err, "snapshot failed for %d: couldn't parse container ID", pid))
		return err
	}

	entry.FileEvent = FileEvent{
		Inode:           inode,
		OverlayNumLower: info.OverlayNumLower,
		MountID:         info.MountID,
		PathnameStr:     pathnameStr,
	}
	// resolve container path with the MountResolver
	entry.FileEvent.ResolveContainerPathWithResolvers(p.resolvers)

	entry.ContainerContext.ID = string(containerID)
	entry.ExecTimestamp = time.Unix(0, proc.CreateTime*int64(time.Millisecond))
	entry.Comm = proc.Name
	entry.PPid = uint32(proc.Ppid)
	entry.TTYName = utils.PidTTY(pid)
	entry.ProcessContext.Pid = pid
	entry.ProcessContext.Tid = pid
	if len(proc.Uids) > 0 {
		entry.ProcessContext.UID = uint32(proc.Uids[0])
	}
	if len(proc.Gids) > 0 {
		entry.ProcessContext.GID = uint32(proc.Gids[0])
	}
	return nil
}

// retrieveInodeInfo fetches inode metadata from kernel space
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

// insertEntry inserts an event in the cache and ensures that the lineage of the new entry is properly updated
func (p *ProcessResolver) insertEntry(pid uint32, entry *ProcessCacheEntry) *ProcessCacheEntry {
	// look for the parent entry to update the processes lineage
	parent, ok := p.entryCache[entry.PPid]
	if ok {
		// if the execve time of entry is not set, then entry was generated by a fork of a process for which
		// we've lost kernel space context (LRU or snapshot). Copy all the data we can from the parent.
		if entry.ExecTimestamp.IsZero() {
			newEntry := parent.Copy()
			// pid, tid, uid, gid, ppid and fork timestamp are the only attributes that we can salvage for sure from entry
			newEntry.Pid = entry.Pid
			newEntry.Tid = entry.Tid
			newEntry.UID = entry.UID
			newEntry.User = entry.User
			newEntry.GID = entry.GID
			newEntry.Group = entry.Group
			newEntry.ForkTimestamp = entry.ForkTimestamp
			newEntry.PPid = entry.PPid
			entry = newEntry
		}

		// inherit the container ID from the parent if necessary. If a container is already running when system-probe
		// starts, the in-kernel process cache will have out of sync container ID values for the processes of that
		// container (the snapshot doesn't update the in-kernel cache with the container IDs). This can also happen if
		// the proc_cache LRU ejects an entry.
		// WARNING: this is why the user space cache should not be used to detect container breakouts. Dedicated
		// in-kernel probes will need to be added.
		if len(parent.ID) > 0 && len(entry.ID) == 0 {
			entry.ID = parent.ID
		}

		// add entry to the children of its parent
		parent.Children[entry.Pid] = entry
	} else {
		if entry.PPid >= 1 {
			// create an entry for the parent, if the parent exists it might be populated later
			parent = NewProcessCacheEntry()
			parent.ProcessContext = ProcessContext{
				Pid: entry.PPid,
			}

			// update lineage
			parent.Children[entry.Pid] = entry
			p.entryCache[entry.PPid] = parent
		}
	}
	entry.Parent = parent
	p.entryCache[pid] = entry

	return entry
}

// DeleteEntry tries to delete an entry in the process cache
func (p *ProcessResolver) DeleteEntry(pid uint32, exitTime time.Time) {
	p.Lock()
	defer p.Unlock()

	// Start by updating the exit timestamp of the pid cache entry
	entry, ok := p.entryCache[pid]
	if !ok {
		return
	}
	entry.ExitTimestamp = exitTime

	// delete the entry and clean up its parents if necessary
	p.recursiveDelete(entry)
}

// recursiveDelete deletes an entry and its parent recursively, if the process can be deleted
func (p *ProcessResolver) recursiveDelete(entry *ProcessCacheEntry) {
	// We cannot delete the entry if the process is still alive
	if entry.ExitTimestamp.IsZero() {
		return
	}

	// We cannot delete the entry if it still has children
	if len(entry.Children) > 0 {
		return
	}

	// Delete the entry
	delete(p.entryCache, entry.Pid)

	// There is nothing left to do if the entry does not have a parent
	if entry.Parent == nil {
		return
	}

	// Delete the reference to the entry from its parent
	delete(entry.Parent.Children, entry.Pid)

	// Check recursively if the parent entry can be deleted
	p.recursiveDelete(entry.Parent)
}

// Resolve returns the cache entry for the given pid
func (p *ProcessResolver) Resolve(pid uint32) *ProcessCacheEntry {
	p.Lock()
	defer p.Unlock()
	entry, exists := p.entryCache[pid]
	if exists {
		return entry
	}

	// fallback to the kernel maps directly, the perf event may be delayed / may have been lost
	if entry = p.resolveWithKernelMaps(pid); entry != nil {
		return entry
	}

	// fallback to /proc, the in-kernel LRU may have deleted the entry
	return p.resolveWithProcfs(pid)
}

func (p *ProcessResolver) resolveWithKernelMaps(pid uint32) *ProcessCacheEntry {
	pidb := make([]byte, 4)
	ebpf.ByteOrder.PutUint32(pidb, pid)

	cookieb, err := p.pidCookieMap.LookupBytes(pidb)
	if err != nil || cookieb == nil {
		return nil
	}

	// first 4 bytes are the actual cookie
	entryb, err := p.procCacheMap.LookupBytes(cookieb[0:4])
	if err != nil || entryb == nil {
		return nil
	}

	entry := NewProcessCacheEntry()
	data := append(entryb, cookieb...)
	if len(data) < 208 {
		// not enough data
		return nil
	}
	read, err := entry.UnmarshalBinary(data, p.resolvers, true)
	if err != nil {
		return nil
	}

	entry.UID = ebpf.ByteOrder.Uint32(data[read : read+4])
	entry.GID = ebpf.ByteOrder.Uint32(data[read+4 : read+8])
	entry.Pid = pid
	entry.Tid = pid

	return p.insertEntry(pid, entry)
}

func (p *ProcessResolver) resolveWithProcfs(pid uint32) *ProcessCacheEntry {
	// check if the process is still alive
	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return nil
	}

	if filledProc := utils.GetFilledProcess(proc); filledProc != nil {
		entry, _ := p.syncCache(filledProc)
		return entry
	}
	return nil
}

// Get returns the cache entry for a specified pid
func (p *ProcessResolver) Get(pid uint32) *ProcessCacheEntry {
	p.RLock()
	defer p.RUnlock()
	entry, exists := p.entryCache[pid]
	if exists {
		return entry
	}
	return nil
}

// Start starts the resolver
func (p *ProcessResolver) Start(ctx context.Context) error {
	// initializes the list of snapshot probes
	for _, id := range snapshotProbeIDs {
		probe, ok := p.probe.manager.GetProbe(id)
		if !ok {
			return errors.Errorf("couldn't find probe %s", id)
		}
		p.snapshotProbes = append(p.snapshotProbes, probe)
	}

	var err error
	if p.inodeInfoMap, err = p.probe.Map("inode_info_cache"); err != nil {
		return err
	}

	if p.procCacheMap, err = p.probe.Map("proc_cache"); err != nil {
		return err
	}

	if p.pidCookieMap, err = p.probe.Map("pid_cache"); err != nil {
		return err
	}

	go p.resync(ctx)

	return nil
}

// resync is used to resync the process cache and ensure that we do not have a memory leak over time
func (p *ProcessResolver) resync(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.cleanupDeadProcesses()
		case <-ctx.Done():
			return
		}
	}
}

// cleanupDeadProcesses is used to cleanup the dead processes cache entries. This is a safety net to avoid a memory leak
// if the tree was not properly cleaned up at runtime.
func (p *ProcessResolver) cleanupDeadProcesses() {
	processes, err := process.Pids()
	if err != nil {
		log.Errorf("failed to list processes for cleanup %v", err)
		return
	}

	// wait a little bit to make sure that there isn't a race with the perf map
	time.Sleep(10 * time.Second)

	var toDelete []uint32

	p.RLock()
	for cachedProc, entry := range p.entryCache {
		if !entry.ExitTimestamp.IsZero() {
			continue
		}

		var alive bool
		for _, proc := range processes {
			if proc == int32(cachedProc) {
				alive = true
			}
		}
		if !alive {
			toDelete = append(toDelete, cachedProc)
		}
	}
	p.RUnlock()

	for _, proc := range toDelete {
		p.DeleteEntry(proc, time.Now())
		time.Sleep(100 * time.Millisecond)
	}
}

// SyncCache snapshots /proc for the provided pid. This method returns true if it updated the process cache.
func (p *ProcessResolver) SyncCache(proc *process.FilledProcess) bool {
	// Only a R lock is necessary to check if the entry exists, but if it exists, we'll update it, so a RW lock is
	// required.
	p.Lock()
	defer p.Unlock()
	_, ret := p.syncCache(proc)
	return ret
}

// syncCache snapshots /proc for the provided pid. This method returns true if it updated the process cache.
func (p *ProcessResolver) syncCache(proc *process.FilledProcess) (*ProcessCacheEntry, bool) {
	pid := uint32(proc.Pid)
	if pid == 0 {
		return nil, false
	}

	// Check if an entry is already in cache for the given pid.
	entry, inCache := p.entryCache[pid]
	if inCache && !entry.ExecTimestamp.IsZero() {
		return nil, false
	}
	if !inCache {
		entry = NewProcessCacheEntry()
	}

	// update the cache entry
	if err := p.enrichEventFromProc(entry, proc); err != nil {
		return nil, false
	}

	entry = p.insertEntry(pid, entry)

	log.Tracef("New process cache entry added: %s %s %d/%d", proc.Name, entry.PathnameStr, pid, entry.Inode)

	// loop through the children of the newly inserted process to propagate the container ID if necessary
	if len(entry.ID) > 0 {
		for _, child := range entry.Children {
			if len(child.ID) == 0 {
				child.ID = entry.ID
			}
		}
	}

	return entry, true
}

// NewProcessResolver returns a new process resolver
func NewProcessResolver(probe *Probe, resolvers *Resolvers) (*ProcessResolver, error) {
	return &ProcessResolver{
		probe:      probe,
		resolvers:  resolvers,
		entryCache: make(map[uint32]*ProcessCacheEntry),
	}, nil
}
