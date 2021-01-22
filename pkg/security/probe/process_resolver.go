// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	lib "github.com/DataDog/ebpf"
	"github.com/hashicorp/golang-lru/simplelru"
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
	PIDCacheSize   int
}

// ProcessResolver resolved process context
type ProcessResolver struct {
	sync.RWMutex
	probe                 *Probe
	resolvers             *Resolvers
	client                *statsd.Client
	inodeInfoMap          *lib.Map
	procCacheMap          *lib.Map
	pidCacheMap           *lib.Map
	bestEffortPidCacheMap *lib.Map
	bestEffortCookiesMap  *lib.Map
	cacheSize             int64
	opts                  ProcessResolverOpts

	entryCache  map[uint32]*ProcessCacheEntry
	cookieCache *simplelru.LRU
}

// SendStats sends process resolver metrics
func (p *ProcessResolver) SendStats() error {
	if err := p.client.Gauge(MetricProcessResolverCacheSize, p.GetCacheSize(), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send process_resolver cache_size metric")
	}

	if err := p.client.Gauge(MetricProcessResolverReferenceCount, p.GetEntryCacheSize(), []string{}, 1.0); err != nil {
		return errors.Wrap(err, "failed to send process_resolver reference_count metric")
	}

	return nil
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
func (p *ProcessResolver) enrichEventFromProc(entry *ProcessCacheEntry, proc *process.Process) error {
	filledProc := utils.GetFilledProcess(proc)
	if filledProc == nil {
		return errors.Errorf("snapshot failed for %d: binary was deleted", proc.Pid)
	}

	pid := uint32(proc.Pid)
	// the provided process is a kernel process if its virtual memory size is null
	isKernelProcess := filledProc.MemInfo.VMS == 0

	if !isKernelProcess {
		// Get process filename and pre-fill the cache
		procExecPath := utils.ProcExePath(proc.Pid)
		pathnameStr, err := os.Readlink(procExecPath)
		if err != nil {
			return errors.Wrapf(err, "snapshot failed for %d: couldn't readlink binary", proc.Pid)
		}
		if pathnameStr == "/ (deleted)" {
			return errors.Errorf("snapshot failed for %d: binary was deleted", proc.Pid)
		}

		// Get the inode of the process binary
		fi, err := os.Stat(procExecPath)
		if err != nil {
			return errors.Wrapf(err, "snapshot failed for %d: couldn't stat binary", proc.Pid)
		}
		stat, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			return errors.Errorf("snapshot failed for %d: couldn't stat binary", proc.Pid)
		}
		inode := stat.Ino

		info, err := p.retrieveInodeInfo(inode)
		if err != nil {
			return errors.Wrapf(err, "snapshot failed for %d: couldn't retrieve inode info", proc.Pid)
		}

		// Retrieve the container ID of the process from /proc
		containerID, err := p.resolvers.ContainerResolver.GetContainerID(pid)
		if err != nil {
			return errors.Wrapf(err, "snapshot failed for %d: couldn't parse container ID", proc.Pid)
		}

		entry.FileEvent = FileEvent{
			Inode:           inode,
			OverlayNumLower: info.OverlayNumLower,
			MountID:         info.MountID,
			PathnameStr:     pathnameStr,
		}
		entry.ContainerContext.ID = string(containerID)
	}

	// resolve container path with the MountResolver
	entry.FileEvent.ResolveContainerPathWithResolvers(p.resolvers)

	entry.ExecTimestamp = time.Unix(0, filledProc.CreateTime*int64(time.Millisecond))
	entry.ForkTimestamp = entry.ExecTimestamp
	entry.Comm = filledProc.Name
	entry.PPid = uint32(filledProc.Ppid)
	entry.TTYName = utils.PidTTY(filledProc.Pid)
	entry.ProcessContext.Pid = pid
	entry.ProcessContext.Tid = pid
	if len(filledProc.Uids) > 0 {
		entry.ProcessContext.UID = uint32(filledProc.Uids[0])
	}
	if len(filledProc.Gids) > 0 {
		entry.ProcessContext.GID = uint32(filledProc.Gids[0])
	}
	_ = entry.ProcessContext.ResolveUserWithResolvers(p.resolvers)
	_ = entry.ProcessContext.ResolveGroupWithResolvers(p.resolvers)
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
		// this shouldn't happen but it is better to exit the prev and let the new one replace it
		prev.Exit(entry.ForkTimestamp)
	}

	parent := p.entryCache[entry.PPid]
	if parent != nil {
		parent.Fork(entry)
	}
	p.entryCache[pid] = entry

	_ = p.client.Count(MetricProcessResolverAdded, 1, []string{}, 1.0)

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

	_ = p.client.Count(MetricProcessResolverAdded, 1, []string{}, 1.0)

	if p.opts.DebugCacheSize {
		atomic.AddInt64(&p.cacheSize, 1)

		runtime.SetFinalizer(entry, func(obj interface{}) {
			atomic.AddInt64(&p.cacheSize, -1)
		})
	}

	if entry.Cookie != 0 {
		p.cookieCache.Add(entry.Cookie, entry)
	}

	return entry
}

func (p *ProcessResolver) deleteEntry(pid uint32, exitTime time.Time) {
	// Start by updating the exit timestamp of the pid cache entry
	entry, ok := p.entryCache[pid]
	if !ok {
		return
	}
	entry.Exit(exitTime)
	delete(p.entryCache, entry.Pid)
}

// DeleteEntry tries to delete an entry in the process cache
func (p *ProcessResolver) DeleteEntry(pid uint32, exitTime time.Time) {
	p.Lock()
	defer p.Unlock()

	p.deleteEntry(pid, exitTime)
}

// Resolve returns the cache entry for the given pid
func (p *ProcessResolver) Resolve(pid uint32, cookie uint32) *ProcessCacheEntry {
	p.Lock()
	defer p.Unlock()

	entry, exists := p.entryCache[pid]
	if exists {
		_ = p.client.Count(MetricProcessResolverCacheHits, 1, []string{"type:cache"}, 1.0)
		return entry
	}

	// fallback to the cookie selection
	if cookie > 0 {
		entryP, exists := p.cookieCache.Get(cookie)
		if exists {
			return entryP.(*ProcessCacheEntry)
		}
	}

	// fallback to the kernel maps directly, the perf event may be delayed / may have been lost
	if entry = p.resolveWithKernelMaps(pid); entry != nil {
		_ = p.client.Count(MetricProcessResolverCacheHits, 1, []string{"type:kernel_maps"}, 1.0)
		return entry
	}

	// fallback to /proc, the in-kernel LRU may have deleted the entry
	if entry = p.resolveWithProcfs(pid); entry != nil {
		_ = p.client.Count(MetricProcessResolverCacheHits, 1, []string{"type:procfs"}, 1.0)
		return entry
	}

	_ = p.client.Count(MetricProcessResolverCacheMiss, 1, []string{}, 1.0)
	return nil
}

func (p *ProcessResolver) resolveWithKernelMaps(pid uint32) *ProcessCacheEntry {
	pidb := make([]byte, 4)
	ebpf.ByteOrder.PutUint32(pidb, pid)

	cookieb, err := p.pidCacheMap.LookupBytes(pidb)
	if err != nil || cookieb == nil {
		cookieb, err = p.bestEffortPidCacheMap.LookupBytes(pidb)
		if err != nil || cookieb == nil {
			return nil
		}
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

	entry, _ := p.syncCache(proc)
	return entry
}

// Get returns the cache entry for a specified pid
func (p *ProcessResolver) Get(pid uint32) *ProcessCacheEntry {
	p.RLock()
	defer p.RUnlock()
	return p.entryCache[pid]
}

// Start starts the resolver
func (p *ProcessResolver) Start(ctx context.Context) error {
	var err error
	if p.inodeInfoMap, err = p.probe.Map("inode_info_cache"); err != nil {
		return err
	}

	if p.procCacheMap, err = p.probe.Map("proc_cache"); err != nil {
		return err
	}

	if p.pidCacheMap, err = p.probe.Map("pid_cache"); err != nil {
		return err
	}

	if p.bestEffortPidCacheMap, err = p.probe.Map("best_effort_pid_cache"); err != nil {
		return err
	}

	if p.bestEffortCookiesMap, err = p.probe.Map("best_effort_cookies"); err != nil {
		return err
	}

	go p.cacheFlush(ctx)

	return nil
}

func (p *ProcessResolver) cacheFlush(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case now := <-ticker.C:
			var pids []uint32

			p.RLock()
			for pid := range p.entryCache {
				pids = append(pids, pid)
			}
			p.RUnlock()

			delEntry := func(pid uint32, exitTime time.Time) {
				p.deleteEntry(pid, now)
				_ = p.client.Count(MetricProcessResolverFlushed, 1, []string{}, 1.0)
			}

			// flush slowly
			for _, pid := range pids {
				if _, err := process.NewProcess(int32(pid)); err != nil {
					// check start time to ensure to not delete a recent pid
					p.Lock()
					if entry := p.entryCache[pid]; entry != nil {
						if tm := entry.ExecTimestamp; !tm.IsZero() && tm.Add(time.Minute).Before(now) {
							delEntry(pid, now)
						} else if tm := entry.ForkTimestamp; !tm.IsZero() && tm.Add(time.Minute).Before(now) {
							delEntry(pid, now)
						}
					}
					p.Unlock()
				}
				time.Sleep(50 * time.Millisecond)
			}
		case <-ctx.Done():
			return
		}
	}
}

// SyncCache snapshots /proc for the provided pid. This method returns true if it updated the process cache.
func (p *ProcessResolver) SyncCache(proc *process.Process) bool {
	// Only a R lock is necessary to check if the entry exists, but if it exists, we'll update it, so a RW lock is
	// required.
	p.Lock()
	defer p.Unlock()
	_, ret := p.syncCache(proc)
	return ret
}

// syncCache snapshots /proc for the provided pid. This method returns true if it updated the process cache.
func (p *ProcessResolver) syncCache(proc *process.Process) (*ProcessCacheEntry, bool) {
	pid := uint32(proc.Pid)

	// Check if an entry is already in cache for the given pid.
	entry := p.entryCache[pid]
	if entry != nil {
		return nil, false
	}

	entry = NewProcessCacheEntry()

	// update the cache entry
	if err := p.enrichEventFromProc(entry, proc); err != nil {
		log.Debug(err)
		return nil, false
	}

	if entry = p.insertForkEntry(pid, entry); entry == nil {
		return nil, false
	}

	log.Tracef("New process cache entry added: %s %s %d/%d", entry.Comm, entry.PathnameStr, pid, entry.Inode)

	return entry, true
}

func (p *ProcessResolver) dumpEntry(writer io.Writer, entry *ProcessCacheEntry, already map[string]bool) {
	for entry != nil {
		label := fmt.Sprintf("%s:%d", entry.Comm, entry.Pid)
		if _, exists := already[label]; !exists {
			if !entry.ExitTimestamp.IsZero() {
				label = "[" + label + "]"
			}

			fmt.Fprintf(writer, `"%d:%s" [label="%s"];`, entry.Pid, entry.Comm, label)
			fmt.Fprintln(writer)

			already[label] = true
		}

		if entry.Ancestor != nil {
			relation := fmt.Sprintf(`"%d:%s" -> "%d:%s";`, entry.Ancestor.Pid, entry.Ancestor.Comm, entry.Pid, entry.Comm)
			if _, exists := already[relation]; !exists {
				fmt.Fprintln(writer, relation)

				already[relation] = true
			}
		}

		entry = entry.Ancestor
	}
}

// Dump create a temp file and dump the cache
func (p *ProcessResolver) Dump() (string, error) {
	dump, err := ioutil.TempFile("/tmp", "process-cache-dump-")
	if err != nil {
		return "", err
	}
	defer dump.Close()

	if err := os.Chmod(dump.Name(), 0400); err != nil {
		return "", err
	}

	p.RLock()
	defer p.RUnlock()

	fmt.Fprintf(dump, "digraph ProcessTree {\n")

	already := make(map[string]bool)
	for _, entry := range p.entryCache {
		p.dumpEntry(dump, entry, already)
	}

	fmt.Fprintf(dump, `}`)

	return dump.Name(), err
}

// GetCacheSize returns the cache size of the process resolver
func (p *ProcessResolver) GetCacheSize() float64 {
	p.RLock()
	defer p.RUnlock()
	return float64(len(p.entryCache))
}

// GetEntryCacheSize returns the cache size of the process resolver
func (p *ProcessResolver) GetEntryCacheSize() float64 {
	return float64(atomic.LoadInt64(&p.cacheSize))
}

// MarkCookieAsBestEffort switches the pid_cache entries with the provided cookie to the best_effort_pid_cache map.
// This will prevent fork events with the provided cookie from being sent over the perf map.
func (p *ProcessResolver) MarkCookieAsBestEffort(cookie uint32) {
	if cookie == 0 {
		// nothing to do
		return
	}

	// mark cookie as best effort
	if err := p.bestEffortCookiesMap.Put(&cookie, ebpf.BytesMapItem([]byte{1})); err != nil {
		log.Tracef("failed to mark cookie %d as best effort", cookie)
		return
	}
}

// NewProcessResolver returns a new process resolver
func NewProcessResolver(probe *Probe, resolvers *Resolvers, client *statsd.Client, opts ProcessResolverOpts) (*ProcessResolver, error) {
	cookieLRU, err := simplelru.NewLRU(opts.PIDCacheSize, nil)
	if err != nil {
		return nil, err
	}
	return &ProcessResolver{
		probe:       probe,
		resolvers:   resolvers,
		client:      client,
		entryCache:  make(map[uint32]*ProcessCacheEntry),
		cookieCache: cookieLRU,
		opts:        opts,
	}, nil
}

// NewProcessResolverOpts returns a new set of process resolver options
func NewProcessResolverOpts(debug bool, pidCacheSize int) ProcessResolverOpts {
	return ProcessResolverOpts{
		DebugCacheSize: debug,
		PIDCacheSize:   pidCacheSize,
	}
}
