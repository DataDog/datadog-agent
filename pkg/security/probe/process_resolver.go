// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	lib "github.com/DataDog/ebpf"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/process"
)

const (
	doForkListInput uint64 = iota
	doForkStructInput
)

// getDoForkInput returns the expected input type of _do_fork, do_fork and kernel_clone
func getDoForkInput(probe *Probe) uint64 {
	if probe.kernelVersion != 0 && probe.kernelVersion >= kernel5_3 {
		return doForkStructInput
	}
	return doForkListInput
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

// ProcessResolverOpts options of resolver
type ProcessResolverOpts struct {
	DebugCacheSize bool
}

// ProcessResolver resolved process context
type ProcessResolver struct {
	sync.RWMutex
	probe        *Probe
	resolvers    *Resolvers
	client       *statsd.Client
	inodeInfoMap *lib.Map
	procCacheMap *lib.Map
	pidCookieMap *lib.Map
	entryCache   map[uint32]*ProcessCacheEntry
	cacheSize    int64
	opts         ProcessResolverOpts
}

// AddForkEntry adds an entry to the local cache and returns the newly created entry
func (p *ProcessResolver) AddForkEntry(pid uint32, entry *ProcessCacheEntry) *ProcessCacheEntry {
	p.Lock()
	defer p.Unlock()
	return p.insertForkEntry(pid, entry)
}

// AddExecEntry adds an entry to the local cache and returns the newly created entry
func (p *ProcessResolver) AddExecEntry(pid uint32, entry *ProcessCacheEntry) *ProcessCacheEntry {
	p.Lock()
	defer p.Unlock()
	return p.insertExecEntry(pid, entry)
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
	entry.ForkTimestamp = entry.ExecTimestamp
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

func (p *ProcessResolver) insertForkEntry(pid uint32, entry *ProcessCacheEntry) *ProcessCacheEntry {
	if prev := p.entryCache[pid]; prev != nil {
		prev.Exit(entry.ForkTimestamp)
	}

	parent := p.entryCache[entry.PPid]
	if parent != nil {
		parent.Fork(entry)
	}
	p.entryCache[pid] = entry

	_ = p.client.Count(MetricPrefix+".process_resolver.added", 1, []string{}, 1.0)

	if p.opts.DebugCacheSize {
		atomic.AddInt64(&p.cacheSize, 1)

		runtime.SetFinalizer(entry, func(obj interface{}) {
			atomic.AddInt64(&p.cacheSize, -1)
		})
	}

	return entry
}

func (p *ProcessResolver) insertExecEntry(pid uint32, entry *ProcessCacheEntry) *ProcessCacheEntry {
	if prev := p.entryCache[pid]; prev != nil {
		prev.Exec(entry)
	}

	p.entryCache[pid] = entry

	_ = p.client.Count(MetricPrefix+".process_resolver.added", 1, []string{}, 1.0)

	if p.opts.DebugCacheSize {
		atomic.AddInt64(&p.cacheSize, 1)

		runtime.SetFinalizer(entry, func(obj interface{}) {
			atomic.AddInt64(&p.cacheSize, -1)
		})
	}

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
	entry.Exit(exitTime)

	delete(p.entryCache, entry.Pid)
}

// Resolve returns the cache entry for the given pid
func (p *ProcessResolver) Resolve(pid uint32) *ProcessCacheEntry {
	p.Lock()
	defer p.Unlock()
	var tags []string
	defer func() {
		_ = p.client.Count(MetricPrefix+".process_resolver.hits", 1, tags, 1.0)
	}()

	entry, exists := p.entryCache[pid]
	if exists {
		tags = append(tags, "type:cache")
		return entry
	}

	// fallback to the kernel maps directly, the perf event may be delayed / may have been lost
	if entry = p.resolveWithKernelMaps(pid); entry != nil {
		tags = append(tags, "type:kernel_maps")
		return entry
	}

	// fallback to /proc, the in-kernel LRU may have deleted the entry
	tags = append(tags, "type:procfs")
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

	if entry.ExecTimestamp.IsZero() {
		return p.insertForkEntry(pid, entry)
	}

	return p.insertExecEntry(pid, entry)
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
	return p.entryCache[pid]
}

// Start starts the resolver
func (p *ProcessResolver) Start() error {
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

	return nil
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

	if entry = p.insertForkEntry(pid, entry); entry == nil {
		return nil, false
	}

	log.Tracef("New process cache entry added: %s %s %d/%d", proc.Name, entry.PathnameStr, pid, entry.Inode)

	return entry, true
}

// GetCacheSize returns the cache size of the process resolver
func (p *ProcessResolver) GetCacheSize() float64 {
	p.RLock()
	defer p.RUnlock()
	return float64(len(p.entryCache))
}

// NewProcessResolver returns a new process resolver
func NewProcessResolver(probe *Probe, resolvers *Resolvers, client *statsd.Client, opts ProcessResolverOpts) (*ProcessResolver, error) {
	return &ProcessResolver{
		probe:      probe,
		resolvers:  resolvers,
		client:     client,
		entryCache: make(map[uint32]*ProcessCacheEntry),
		opts:       opts,
	}, nil
}
