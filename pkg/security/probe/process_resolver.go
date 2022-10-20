// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"container/list"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/DataDog/gopsutil/process"
	lib "github.com/cilium/ebpf"
	"github.com/hashicorp/golang-lru/simplelru"
	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	doForkListInput uint64 = iota
	doForkStructInput
)

const (
	snapshotting = iota
	snapshotted
)

const (
	procResolveMaxDepth = 16
	maxArgsEnvResidents = 1024
	maxParallelArgsEnvs = 512 // == number of parallel starting processes
)

func getAttr2(probe *Probe) uint64 {
	if probe.kernelVersion.IsRH7Kernel() {
		return 1
	}
	return 0
}

// getDoForkInput returns the expected input type of _do_fork, do_fork and kernel_clone
func getDoForkInput(probe *Probe) uint64 {
	if probe.kernelVersion.Code != 0 && probe.kernelVersion.Code >= kernel.Kernel5_3 {
		return doForkStructInput
	}
	return doForkListInput
}

// getCGroupWriteConstants returns the value of the constant used to determine how cgroups should be captured in kernel
// space
func getCGroupWriteConstants() manager.ConstantEditor {
	cgroupWriteConst := uint64(1)
	kv, err := kernel.NewKernelVersion()
	if err == nil {
		if kv.IsRH7Kernel() {
			cgroupWriteConst = 2
		}
	}

	return manager.ConstantEditor{
		Name:  "cgroup_write_type",
		Value: cgroupWriteConst,
	}
}

// ProcessResolverOpts options of resolver
type ProcessResolverOpts struct {
	envsWithValue map[string]bool
}

// ProcessResolver resolved process context
type ProcessResolver struct {
	sync.RWMutex
	state            *atomic.Int64
	probe            *Probe
	resolvers        *Resolvers
	execFileCacheMap *lib.Map
	procCacheMap     *lib.Map
	pidCacheMap      *lib.Map
	cacheSize        *atomic.Int64
	opts             ProcessResolverOpts

	// stats
	hitsStats      map[string]*atomic.Int64
	missStats      *atomic.Int64
	addedEntries   *atomic.Int64
	flushedEntries *atomic.Int64
	pathErrStats   *atomic.Int64
	argsTruncated  *atomic.Int64
	argsSize       *atomic.Int64
	envsTruncated  *atomic.Int64
	envsSize       *atomic.Int64

	entryCache    map[uint32]*model.ProcessCacheEntry
	argsEnvsCache *simplelru.LRU

	argsEnvsPool          *ArgsEnvsPool
	processCacheEntryPool *ProcessCacheEntryPool

	exitedQueue []uint32
}

// ArgsEnvsPool defines a pool for args/envs allocations
type ArgsEnvsPool struct {
	lock sync.RWMutex
	pool *sync.Pool

	// entries that wont be release to the pool
	maxResidents   int
	totalResidents int
	freeResidents  *list.List
}

// Get returns a cache entry
func (a *ArgsEnvsPool) Get() *model.ArgsEnvsCacheEntry {
	a.lock.RLock()
	defer a.lock.RUnlock()

	// first try from resident pool
	if el := a.freeResidents.Front(); el != nil {
		entry := el.Value.(*model.ArgsEnvsCacheEntry)
		a.freeResidents.Remove(el)
		return entry
	}

	return a.pool.Get().(*model.ArgsEnvsCacheEntry)
}

// GetFrom returns a new entry with value from the given entry
func (a *ArgsEnvsPool) GetFrom(event *model.ArgsEnvsEvent) *model.ArgsEnvsCacheEntry {
	a.lock.RLock()
	defer a.lock.RUnlock()

	entry := a.Get()

	entry.Size = event.ArgsEnvs.Size
	entry.ValuesRaw = make([]byte, entry.Size)
	copy(entry.ValuesRaw, event.ArgsEnvs.ValuesRaw[:])

	return entry
}

// Put returns a cache entry to the pool
func (a *ArgsEnvsPool) Put(entry *model.ArgsEnvsCacheEntry) {
	a.lock.Lock()
	defer a.lock.Unlock()

	if entry.Container != nil {
		// from the residents list
		a.freeResidents.MoveToBack(entry.Container)
	} else if a.totalResidents < a.maxResidents {
		// still some places so we can create a new node
		entry.Container = &list.Element{Value: entry}
		a.totalResidents++

		a.freeResidents.MoveToBack(entry.Container)
	} else {
		a.pool.Put(entry)
	}
}

// NewArgsEnvsPool returns a new ArgsEnvEntry pool
func NewArgsEnvsPool(maxResident int) *ArgsEnvsPool {
	ap := ArgsEnvsPool{
		pool:          &sync.Pool{},
		maxResidents:  maxResident,
		freeResidents: list.New(),
	}

	ap.pool.New = func() interface{} {
		return model.NewArgsEnvsCacheEntry(ap.Put)
	}

	return &ap
}

// ProcessCacheEntryPool defines a pool for process entry allocations
type ProcessCacheEntryPool struct {
	pool *sync.Pool
}

// Get returns a cache entry
func (p *ProcessCacheEntryPool) Get() *model.ProcessCacheEntry {
	return p.pool.Get().(*model.ProcessCacheEntry)
}

// Put returns a cache entry
func (p *ProcessCacheEntryPool) Put(pce *model.ProcessCacheEntry) {
	pce.Reset()
	p.pool.Put(pce)
}

// NewProcessCacheEntryPool returns a new ProcessCacheEntryPool pool
func NewProcessCacheEntryPool(p *ProcessResolver) *ProcessCacheEntryPool {
	pcep := ProcessCacheEntryPool{pool: &sync.Pool{}}

	pcep.pool.New = func() interface{} {
		return model.NewProcessCacheEntry(func(pce *model.ProcessCacheEntry) {
			if pce.Ancestor != nil {
				pce.Ancestor.Release()
			}

			if pce.ArgsEntry != nil && pce.ArgsEntry.ArgsEnvsCacheEntry != nil {
				pce.ArgsEntry.ArgsEnvsCacheEntry.Release()
			}
			if pce.EnvsEntry != nil && pce.EnvsEntry.ArgsEnvsCacheEntry != nil {
				pce.EnvsEntry.ArgsEnvsCacheEntry.Release()
			}

			p.cacheSize.Dec()

			pcep.Put(pce)
		})
	}

	return &pcep
}

// DequeueExited dequeue exited process
func (p *ProcessResolver) DequeueExited() {
	p.Lock()
	defer p.Unlock()

	delEntry := func(pid uint32, exitTime time.Time) {
		p.deleteEntry(pid, exitTime)
		p.flushedEntries.Inc()
	}

	now := time.Now()
	for _, pid := range p.exitedQueue {
		entry := p.entryCache[pid]
		if entry == nil {
			continue
		}

		if tm := entry.ExecTime; !tm.IsZero() && tm.Add(time.Minute).Before(now) {
			delEntry(pid, now)
		} else if tm := entry.ForkTime; !tm.IsZero() && tm.Add(time.Minute).Before(now) {
			delEntry(pid, now)
		} else if entry.ForkTime.IsZero() && entry.ExecTime.IsZero() {
			delEntry(pid, now)
		}
	}

	p.exitedQueue = p.exitedQueue[0:0]
}

// NewProcessCacheEntry returns a new process cache entry
func (p *ProcessResolver) NewProcessCacheEntry(pidContext model.PIDContext) *model.ProcessCacheEntry {
	entry := p.processCacheEntryPool.Get()
	entry.PIDContext = pidContext
	entry.Cookie = eval.NewCookie()
	return entry
}

// SendStats sends process resolver metrics
func (p *ProcessResolver) SendStats() error {
	if err := p.probe.statsdClient.Gauge(metrics.MetricProcessResolverCacheSize, p.GetCacheSize(), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send process_resolver cache_size metric: %w", err)
	}

	if err := p.probe.statsdClient.Gauge(metrics.MetricProcessResolverReferenceCount, p.GetEntryCacheSize(), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send process_resolver reference_count metric: %w", err)
	}

	for _, resolutionType := range metrics.AllTypesTags {
		if count := p.hitsStats[resolutionType].Swap(0); count > 0 {
			if err := p.probe.statsdClient.Count(metrics.MetricProcessResolverHits, count, []string{resolutionType}, 1.0); err != nil {
				return fmt.Errorf("failed to send process_resolver with `%s` metric: %w", resolutionType, err)
			}
		}
	}

	if count := p.missStats.Swap(0); count > 0 {
		if err := p.probe.statsdClient.Count(metrics.MetricProcessResolverMiss, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send process_resolver misses metric: %w", err)
		}
	}

	if count := p.addedEntries.Swap(0); count > 0 {
		if err := p.probe.statsdClient.Count(metrics.MetricProcessResolverAdded, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send process_resolver added entries metric: %w", err)
		}
	}

	if count := p.flushedEntries.Swap(0); count > 0 {
		if err := p.probe.statsdClient.Count(metrics.MetricProcessResolverFlushed, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send process_resolver flushed entries metric: %w", err)
		}
	}

	if count := p.pathErrStats.Swap(0); count > 0 {
		if err := p.probe.statsdClient.Count(metrics.MetricProcessResolverPathError, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send process_resolver path error metric: %w", err)
		}
	}

	if count := p.argsTruncated.Swap(0); count > 0 {
		if err := p.probe.statsdClient.Count(metrics.MetricProcessResolverArgsTruncated, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send args truncated metric: %w", err)
		}
	}

	if count := p.argsSize.Swap(0); count > 0 {
		if err := p.probe.statsdClient.Count(metrics.MetricProcessResolverArgsSize, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send args size metric: %w", err)
		}
	}

	if count := p.envsTruncated.Swap(0); count > 0 {
		if err := p.probe.statsdClient.Count(metrics.MetricProcessResolverEnvsTruncated, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send envs truncated metric: %w", err)
		}
	}

	if count := p.envsSize.Swap(0); count > 0 {
		if err := p.probe.statsdClient.Count(metrics.MetricProcessResolverEnvsSize, count, []string{}, 1.0); err != nil {
			return fmt.Errorf("failed to send envs size metric: %w", err)
		}
	}

	return nil
}

// UpdateArgsEnvs updates arguments or environment variables of the given id
func (p *ProcessResolver) UpdateArgsEnvs(event *model.ArgsEnvsEvent) {
	entry := p.argsEnvsPool.GetFrom(event)
	if e, found := p.argsEnvsCache.Get(event.ID); found {
		list := e.(*model.ArgsEnvsCacheEntry)
		list.Append(entry)
	} else {
		p.argsEnvsCache.Add(event.ID, entry)
	}
}

// AddForkEntry adds an entry to the local cache and returns the newly created entry
func (p *ProcessResolver) AddForkEntry(entry *model.ProcessCacheEntry) {
	p.Lock()
	defer p.Unlock()

	p.insertForkEntry(entry)
}

// AddExecEntry adds an entry to the local cache and returns the newly created entry
func (p *ProcessResolver) AddExecEntry(entry *model.ProcessCacheEntry) {
	p.Lock()
	defer p.Unlock()

	p.insertExecEntry(entry)
}

// enrichEventFromProc uses /proc to enrich a ProcessCacheEntry with additional metadata
func (p *ProcessResolver) enrichEventFromProc(entry *model.ProcessCacheEntry, proc *process.Process, filledProc *process.FilledProcess) error {
	// the provided process is a kernel process if its virtual memory size is null
	if filledProc.MemInfo.VMS == 0 {
		return fmt.Errorf("cannot snapshot kernel threads")
	}
	pid := uint32(proc.Pid)

	// Get process filename and pre-fill the cache
	procExecPath := utils.ProcExePath(proc.Pid)
	pathnameStr, err := os.Readlink(procExecPath)
	if err != nil {
		return fmt.Errorf("snapshot failed for %d: couldn't readlink binary: %w", proc.Pid, err)
	}
	if pathnameStr == "/ (deleted)" {
		return fmt.Errorf("snapshot failed for %d: binary was deleted", proc.Pid)
	}

	// Get the file fields of the process binary
	info, err := p.retrieveExecFileFields(procExecPath)
	if err != nil {
		return fmt.Errorf("snapshot failed for %d: couldn't retrieve inode info: %w", proc.Pid, err)
	}

	// Retrieve the container ID of the process from /proc
	containerID, err := p.resolvers.ContainerResolver.GetContainerID(pid)
	if err != nil {
		return fmt.Errorf("snapshot failed for %d: couldn't parse container ID: %w", proc.Pid, err)
	}

	entry.FileEvent.FileFields = *info
	entry.FileEvent.SetPathnameStr(pathnameStr)
	entry.FileEvent.SetBasenameStr(path.Base(pathnameStr))

	entry.Process.ContainerID = string(containerID)
	// resolve container path with the MountResolver
	entry.FileEvent.Filesystem = p.resolvers.MountResolver.GetFilesystem(entry.Process.FileEvent.MountID, entry.Process.Pid)

	entry.ExecTime = time.Unix(0, filledProc.CreateTime*int64(time.Millisecond))
	entry.ForkTime = entry.ExecTime
	entry.Comm = filledProc.Name
	entry.PPid = uint32(filledProc.Ppid)
	entry.TTYName = utils.PidTTY(filledProc.Pid)
	entry.ProcessContext.Pid = pid
	entry.ProcessContext.Tid = pid
	if len(filledProc.Uids) >= 4 {
		entry.Credentials.UID = uint32(filledProc.Uids[0])
		entry.Credentials.EUID = uint32(filledProc.Uids[1])
		entry.Credentials.FSUID = uint32(filledProc.Uids[3])
	}
	if len(filledProc.Gids) >= 4 {
		entry.Credentials.GID = uint32(filledProc.Gids[0])
		entry.Credentials.EGID = uint32(filledProc.Gids[1])
		entry.Credentials.FSGID = uint32(filledProc.Gids[3])
	}
	entry.Credentials.CapEffective, entry.Credentials.CapPermitted, err = utils.CapEffCapEprm(proc.Pid)
	if err != nil {
		return fmt.Errorf("snapshot failed for %d: couldn't parse kernel capabilities: %w", proc.Pid, err)
	}
	p.SetProcessUsersGroups(entry)

	// args and envs
	if len(filledProc.Cmdline) > 0 {
		entry.ArgsEntry = &model.ArgsEntry{
			Values: filledProc.Cmdline,
		}
	}

	if envs, err := utils.EnvVars(proc.Pid); err == nil {
		entry.EnvsEntry = &model.EnvsEntry{
			Values: envs,
		}
	}

	if parent := p.entryCache[entry.PPid]; parent != nil {
		if parent.Equals(entry) {
			parent.ShareArgsEnvs(entry)
		}
	}

	// Heuristic to detect likely interpreter event
	// Cannot detect when a script if as follows:
	// perl <<__HERE__
	//#!/usr/bin/perl
	//
	//sleep 10;
	//
	//print "Hello from Perl\n";
	//__HERE__
	// Because the entry only has 1 argument (perl in this case). But can detect when a script is as follows:
	//cat << EOF > perlscript.pl
	//#!/usr/bin/perl
	//
	//sleep 15;
	//
	//print "Hello from Perl\n";
	//
	//EOF
	if valueCount := len(entry.ArgsEntry.Values); valueCount > 1 {
		firstArg := entry.ArgsEntry.Values[0]
		lastArg := entry.ArgsEntry.Values[valueCount-1]
		// Example result: comm value: pyscript.py | args: [/usr/bin/python3 ./pyscript.py]
		if path.Base(lastArg) == entry.Comm && path.IsAbs(firstArg) {
			entry.LinuxBinprm.FileEvent = entry.FileEvent
		}
	}

	if !entry.HasInterpreter() {
		// mark it as resolved to avoid abnormal path later in the call flow
		entry.LinuxBinprm.FileEvent.SetPathnameStr("")
		entry.LinuxBinprm.FileEvent.SetBasenameStr("")
	}

	// add netns
	entry.NetNS, _ = utils.NetNSPathFromPid(pid).GetProcessNetworkNamespace()

	if p.probe.config.NetworkEnabled {
		// snapshot pid routes in kernel space
		_, _ = proc.OpenFiles()
	}

	return nil
}

// retrieveExecFileFields fetches inode metadata from kernel space
func (p *ProcessResolver) retrieveExecFileFields(procExecPath string) (*model.FileFields, error) {
	fi, err := os.Stat(procExecPath)
	if err != nil {
		return nil, fmt.Errorf("snapshot failed for `%s`: couldn't stat binary: %w", procExecPath, err)
	}
	stat, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, fmt.Errorf("snapshot failed for `%s`: couldn't stat binary", procExecPath)
	}
	inode := stat.Ino

	inodeb := make([]byte, 8)
	model.ByteOrder.PutUint64(inodeb, inode)

	data, err := p.execFileCacheMap.LookupBytes(inodeb)
	if err != nil {
		return nil, fmt.Errorf("unable to get filename for inode `%d`: %v", inode, err)
	}

	var fileFields model.FileFields
	if _, err := fileFields.UnmarshalBinary(data); err != nil {
		return nil, fmt.Errorf("unable to unmarshal entry for inode `%d`", inode)
	}

	if fileFields.Inode == 0 {
		return nil, errors.New("not found")
	}

	return &fileFields, nil
}

func (p *ProcessResolver) insertEntry(entry, prev *model.ProcessCacheEntry) {
	p.entryCache[entry.Pid] = entry
	entry.Retain()

	if prev != nil {
		prev.Release()
	}

	p.addedEntries.Inc()
	p.cacheSize.Inc()
}

func (p *ProcessResolver) insertForkEntry(entry *model.ProcessCacheEntry) {
	prev := p.entryCache[entry.Pid]
	if prev != nil {
		// this shouldn't happen but it is better to exit the prev and let the new one replace it
		prev.Exit(entry.ForkTime)
	}

	parent := p.entryCache[entry.PPid]
	if parent == nil && entry.PPid >= 1 {
		parent = p.resolve(entry.PPid, entry.PPid)
	}

	if parent != nil {
		parent.Fork(entry)
	}

	p.insertEntry(entry, prev)
}

func (p *ProcessResolver) insertExecEntry(entry *model.ProcessCacheEntry) {
	prev := p.entryCache[entry.Pid]
	if prev != nil {
		prev.Exec(entry)
	}

	p.insertEntry(entry, prev)
}

func (p *ProcessResolver) deleteEntry(pid uint32, exitTime time.Time) {
	// Start by updating the exit timestamp of the pid cache entry
	entry, ok := p.entryCache[pid]
	if !ok {
		return
	}
	entry.Exit(exitTime)

	delete(p.entryCache, entry.Pid)
	entry.Release()
}

// DeleteEntry tries to delete an entry in the process cache
func (p *ProcessResolver) DeleteEntry(pid uint32, exitTime time.Time) {
	p.Lock()
	defer p.Unlock()

	p.deleteEntry(pid, exitTime)
}

// Resolve returns the cache entry for the given pid
func (p *ProcessResolver) Resolve(pid, tid uint32) *model.ProcessCacheEntry {
	p.Lock()
	defer p.Unlock()

	return p.resolve(pid, tid)
}

func (p *ProcessResolver) resolve(pid, tid uint32) *model.ProcessCacheEntry {
	if entry := p.resolveFromCache(pid, tid); entry != nil {
		p.hitsStats[metrics.CacheTag].Inc()
		return entry
	}

	if p.state.Load() != snapshotted {
		return nil
	}

	// fallback to the kernel maps directly, the perf event may be delayed / may have been lost
	if entry := p.resolveFromKernelMaps(pid, tid); entry != nil {
		p.hitsStats[metrics.KernelMapsTag].Inc()
		return entry
	}

	// fallback to /proc, the in-kernel LRU may have deleted the entry
	if entry := p.resolveFromProcfs(pid, procResolveMaxDepth); entry != nil {
		p.hitsStats[metrics.ProcFSTag].Inc()
		return entry
	}

	p.missStats.Inc()
	return nil
}

// SetProcessPath resolves process file path
func (p *ProcessResolver) SetProcessPath(fileEvent *model.FileEvent, ctx *model.PIDContext) (string, error) {

	if fileEvent.Inode == 0 || fileEvent.MountID == 0 {
		fileEvent.SetPathnameStr("")
		fileEvent.SetBasenameStr("")

		p.pathErrStats.Inc()
		return "", &ErrInvalidKeyPath{Inode: fileEvent.Inode, MountID: fileEvent.MountID}
	}

	pathnameStr, err := p.resolvers.resolveFileFieldsPath(&fileEvent.FileFields, ctx)
	if err != nil {
		fileEvent.SetPathnameStr("")
		fileEvent.SetBasenameStr("")

		p.pathErrStats.Inc()

		return "", &ErrInvalidKeyPath{Inode: fileEvent.Inode, MountID: fileEvent.MountID}
	}

	fileEvent.SetPathnameStr(pathnameStr)
	fileEvent.SetBasenameStr(path.Base(fileEvent.PathnameStr))

	return fileEvent.PathnameStr, nil
}

func isBusybox(pathname string) bool {
	return pathname == "/bin/busybox" || pathname == "/usr/bin/busybox"
}

// SetProcessSymlink resolves process file symlink path
func (p *ProcessResolver) SetProcessSymlink(entry *model.ProcessCacheEntry) {
	// TODO: busybox workaround only for now
	if isBusybox(entry.FileEvent.PathnameStr) {
		arg0, _ := p.GetProcessArgv0(&entry.Process)
		base := path.Base(arg0)

		entry.SymlinkPathnameStr[0] = "/bin/" + base
		entry.SymlinkPathnameStr[1] = "/usr/bin/" + base

		entry.SymlinkBasenameStr = base
	}
}

// SetProcessFilesystem resolves process file system
func (p *ProcessResolver) SetProcessFilesystem(entry *model.ProcessCacheEntry) string {
	if entry.FileEvent.MountID != 0 {
		entry.FileEvent.Filesystem = p.resolvers.MountResolver.GetFilesystem(entry.FileEvent.MountID, entry.Pid)
	}

	return entry.FileEvent.Filesystem
}

// ApplyBootTime realign timestamp from the boot time
func (p *ProcessResolver) ApplyBootTime(entry *model.ProcessCacheEntry) {
	entry.ExecTime = p.resolvers.TimeResolver.ApplyBootTime(entry.ExecTime)
	entry.ForkTime = p.resolvers.TimeResolver.ApplyBootTime(entry.ForkTime)
	entry.ExitTime = p.resolvers.TimeResolver.ApplyBootTime(entry.ExitTime)
}

// ResolveFromCache resolves cache entry from the cache
func (p *ProcessResolver) ResolveFromCache(pid, tid uint32) *model.ProcessCacheEntry {
	p.Lock()
	defer p.Unlock()
	return p.resolveFromCache(pid, tid)
}

func (p *ProcessResolver) resolveFromCache(pid, tid uint32) *model.ProcessCacheEntry {
	entry, exists := p.entryCache[pid]
	if !exists {
		return nil
	}

	// make to update the tid with the that triggers the resolution
	entry.Tid = tid

	return entry
}

// ResolveNewProcessCacheEntry resolves the context fields of a new process cache entry parsed from kernel data
func (p *ProcessResolver) ResolveNewProcessCacheEntry(entry *model.ProcessCacheEntry) error {
	if _, err := p.SetProcessPath(&entry.FileEvent, &entry.PIDContext); err != nil {
		return fmt.Errorf("failed to resolve exec path: %w", err)
	}

	if entry.HasInterpreter() {
		if _, err := p.SetProcessPath(&entry.LinuxBinprm.FileEvent, &entry.PIDContext); err != nil {
			return fmt.Errorf("failed to resolve interpreter path: %w", err)
		}
	} else {
		// mark it as resolved to avoid abnormal path later in the call flow
		entry.LinuxBinprm.FileEvent.SetPathnameStr("")
		entry.LinuxBinprm.FileEvent.SetBasenameStr("")
	}

	p.SetProcessArgs(entry)
	p.SetProcessEnvs(entry)
	p.SetProcessFilesystem(entry)
	p.SetProcessTTY(entry)
	p.SetProcessUsersGroups(entry)
	p.ApplyBootTime(entry)

	p.SetProcessSymlink(entry)

	return nil
}

// ResolveFromKernelMaps resolves the entry from the kernel maps
func (p *ProcessResolver) ResolveFromKernelMaps(pid, tid uint32) *model.ProcessCacheEntry {
	p.Lock()
	defer p.Unlock()
	return p.resolveFromKernelMaps(pid, tid)
}

func (p *ProcessResolver) resolveFromKernelMaps(pid, tid uint32) *model.ProcessCacheEntry {
	pidb := make([]byte, 4)
	model.ByteOrder.PutUint32(pidb, pid)

	pidCache, err := p.pidCacheMap.LookupBytes(pidb)
	if err != nil || pidCache == nil {
		return nil
	}

	// first 4 bytes are the actual cookie
	procCache, err := p.procCacheMap.LookupBytes(pidCache[0:4])
	if err != nil || procCache == nil {
		return nil
	}

	entry := p.NewProcessCacheEntry(model.PIDContext{Pid: pid, Tid: tid})

	var cc model.ContainerContext
	read, err := cc.UnmarshalBinary(procCache)
	if err != nil {
		return nil
	}
	entry.ContainerID = cc.ID

	if _, err := entry.UnmarshalProcEntryBinary(procCache[read:]); err != nil {
		return nil
	}

	if _, err := entry.UnmarshalPidCacheBinary(pidCache); err != nil {
		return nil
	}

	// resolve paths and other context fields
	if err = p.ResolveNewProcessCacheEntry(entry); err != nil {
		return nil
	}

	// If we fall back to the kernel maps for a process in a container that was already running when the agent
	// started, the kernel space container ID will be empty even though the process is inside a container. Since there
	// is no insurance that the parent of this process is still running, we can't use our user space cache to check if
	// the parent is in a container. In other words, we have to fall back to /proc to query the container ID of the
	// process.
	if entry.ContainerID == "" {
		containerID, err := p.resolvers.ContainerResolver.GetContainerID(pid)
		if err == nil {
			entry.ContainerID = string(containerID)
		}
	}

	if entry.ExecTime.IsZero() {
		p.insertForkEntry(entry)
		return entry
	}

	p.insertExecEntry(entry)
	return entry
}

// IsKThread returns whether given pids are from kthreads
func IsKThread(ppid, pid uint32) bool {
	return ppid == 2 || pid == 2
}

// ResolveFromProcfs resolves the entry from procfs
func (p *ProcessResolver) ResolveFromProcfs(pid uint32) *model.ProcessCacheEntry {
	p.Lock()
	defer p.Unlock()
	return p.resolveFromProcfs(pid, procResolveMaxDepth)
}

func (p *ProcessResolver) resolveFromProcfs(pid uint32, maxDepth int) *model.ProcessCacheEntry {
	if maxDepth < 1 || pid == 0 {
		return nil
	}

	var ppid uint32
	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return nil
	}

	filledProc := utils.GetFilledProcess(proc)
	if filledProc == nil {
		return nil
	}

	// ignore kthreads
	if IsKThread(uint32(filledProc.Ppid), uint32(filledProc.Pid)) {
		return nil
	}

	entry, inserted := p.syncCache(proc, filledProc)
	if entry != nil {
		// consider kworker processes with 0 as ppid
		entry.IsKworker = filledProc.Ppid == 0
	}

	ppid = uint32(filledProc.Ppid)

	parent := p.resolveFromProcfs(ppid, maxDepth-1)
	if inserted && entry != nil && parent != nil {
		if parent.Equals(entry) {
			entry.SetParent(parent)
		} else {
			entry.SetAncestor(parent)
		}
	}

	return entry
}

// SetProcessArgs set arguments to cache entry
func (p *ProcessResolver) SetProcessArgs(pce *model.ProcessCacheEntry) {
	if e, found := p.argsEnvsCache.Get(pce.ArgsID); found {
		if pce.ArgsTruncated {
			p.argsTruncated.Inc()
		}

		entry := e.(*model.ArgsEnvsCacheEntry)
		p.argsSize.Add(int64(entry.TotalSize))

		pce.ArgsEntry = &model.ArgsEntry{
			ArgsEnvsCacheEntry: entry,
		}

		// attach to a process thus retain the head of the chain
		// note: only the head of the list is retained and when released
		// the whole list will be released
		pce.ArgsEntry.ArgsEnvsCacheEntry.Retain()

		// no need to keep it in LRU now as attached to a process
		p.argsEnvsCache.Remove(pce.ArgsID)
	}
}

// GetProcessArgv returns the args of the event as an array
func (p *ProcessResolver) GetProcessArgv(pr *model.Process) ([]string, bool) {
	if pr.ArgsEntry == nil {
		return nil, false
	}

	argv, truncated := pr.ArgsEntry.ToArray()
	if len(argv) > 0 {
		argv = argv[1:]
	}

	return argv, pr.ArgsTruncated || truncated
}

// GetProcessArgv0 returns the first arg of the event
func (p *ProcessResolver) GetProcessArgv0(pr *model.Process) (string, bool) {
	if pr.ArgsEntry == nil {
		return "", false
	}

	argv, truncated := pr.ArgsEntry.ToArray()
	if len(argv) > 0 {
		return argv[0], pr.ArgsTruncated || truncated
	}

	return "", pr.ArgsTruncated || truncated
}

// GetProcessScrubbedArgv returns the scrubbed args of the event as an array
func (p *ProcessResolver) GetProcessScrubbedArgv(pr *model.Process) ([]string, bool) {
	if pr.ScrubbedArgvResolved {
		return pr.ScrubbedArgv, pr.ScrubbedArgsTruncated
	}

	argv, truncated := p.GetProcessArgv(pr)

	if p.probe.scrubber != nil {
		argv, _ = p.probe.scrubber.ScrubCommand(argv)
	}

	pr.ScrubbedArgv = argv
	pr.ScrubbedArgsTruncated = truncated
	pr.ScrubbedArgvResolved = true

	return argv, truncated
}

// SetProcessEnvs set envs to cache entry
func (p *ProcessResolver) SetProcessEnvs(pce *model.ProcessCacheEntry) {
	if e, found := p.argsEnvsCache.Get(pce.EnvsID); found {
		if pce.EnvsTruncated {
			p.envsTruncated.Inc()
		}

		entry := e.(*model.ArgsEnvsCacheEntry)
		p.envsSize.Add(int64(entry.TotalSize))

		pce.EnvsEntry = &model.EnvsEntry{
			ArgsEnvsCacheEntry: entry,
		}

		// attach to a process thus retain the head of the chain
		// note: only the head of the list is retained and when released
		// the whole list will be released
		pce.EnvsEntry.ArgsEnvsCacheEntry.Retain()

		// no need to keep it in LRU now as attached to a process
		p.argsEnvsCache.Remove(pce.ArgsID)
	}
}

// GetProcessEnvs returns the envs of the event
func (p *ProcessResolver) GetProcessEnvs(pr *model.Process) ([]string, bool) {
	if pr.EnvsEntry == nil {
		return nil, false
	}

	keys, truncated := pr.EnvsEntry.FilterEnvs(p.opts.envsWithValue)

	return keys, pr.EnvsTruncated || truncated
}

// GetProcessEnvp returns the envs of the event with their values
func (p *ProcessResolver) GetProcessEnvp(pr *model.Process) ([]string, bool) {
	if pr.EnvsEntry == nil {
		return nil, false
	}

	envp, truncated := pr.EnvsEntry.ToArray()

	return envp, pr.EnvsTruncated || truncated
}

// SetProcessTTY resolves TTY and cache the result
func (p *ProcessResolver) SetProcessTTY(pce *model.ProcessCacheEntry) string {
	if pce.TTYName == "" {
		tty := utils.PidTTY(int32(pce.Pid))
		pce.TTYName = tty
	}
	return pce.TTYName
}

// SetProcessUsersGroups resolves and set users and groups
func (p *ProcessResolver) SetProcessUsersGroups(pce *model.ProcessCacheEntry) {
	pce.User, _ = p.resolvers.UserGroupResolver.ResolveUser(int(pce.Credentials.UID))
	pce.EUser, _ = p.resolvers.UserGroupResolver.ResolveUser(int(pce.Credentials.EUID))
	pce.FSUser, _ = p.resolvers.UserGroupResolver.ResolveUser(int(pce.Credentials.FSUID))

	pce.Group, _ = p.resolvers.UserGroupResolver.ResolveGroup(int(pce.Credentials.GID))
	pce.EGroup, _ = p.resolvers.UserGroupResolver.ResolveGroup(int(pce.Credentials.EGID))
	pce.FSGroup, _ = p.resolvers.UserGroupResolver.ResolveGroup(int(pce.Credentials.FSGID))
}

// Get returns the cache entry for a specified pid
func (p *ProcessResolver) Get(pid uint32) *model.ProcessCacheEntry {
	p.RLock()
	defer p.RUnlock()
	return p.entryCache[pid]
}

// UpdateUID updates the credentials of the provided pid
func (p *ProcessResolver) UpdateUID(pid uint32, e *Event) {
	if e.ProcessContext.Pid != e.ProcessContext.Tid {
		return
	}

	p.Lock()
	defer p.Unlock()
	entry := p.entryCache[pid]
	if entry != nil {
		entry.Credentials.UID = e.SetUID.UID
		entry.Credentials.User = e.ResolveSetuidUser(&e.SetUID)
		entry.Credentials.EUID = e.SetUID.EUID
		entry.Credentials.EUser = e.ResolveSetuidEUser(&e.SetUID)
		entry.Credentials.FSUID = e.SetUID.FSUID
		entry.Credentials.FSUser = e.ResolveSetuidFSUser(&e.SetUID)
	}
}

// UpdateGID updates the credentials of the provided pid
func (p *ProcessResolver) UpdateGID(pid uint32, e *Event) {
	if e.ProcessContext.Pid != e.ProcessContext.Tid {
		return
	}

	p.Lock()
	defer p.Unlock()
	entry := p.entryCache[pid]
	if entry != nil {
		entry.Credentials.GID = e.SetGID.GID
		entry.Credentials.Group = e.ResolveSetgidGroup(&e.SetGID)
		entry.Credentials.EGID = e.SetGID.EGID
		entry.Credentials.EGroup = e.ResolveSetgidEGroup(&e.SetGID)
		entry.Credentials.FSGID = e.SetGID.FSGID
		entry.Credentials.FSGroup = e.ResolveSetgidFSGroup(&e.SetGID)
	}
}

// UpdateCapset updates the credentials of the provided pid
func (p *ProcessResolver) UpdateCapset(pid uint32, e *Event) {
	if e.ProcessContext.Pid != e.ProcessContext.Tid {
		return
	}

	p.Lock()
	defer p.Unlock()
	entry := p.entryCache[pid]
	if entry != nil {
		entry.Credentials.CapEffective = e.Capset.CapEffective
		entry.Credentials.CapPermitted = e.Capset.CapPermitted
	}
}

// Start starts the resolver
func (p *ProcessResolver) Start(ctx context.Context) error {
	var err error
	if p.execFileCacheMap, err = p.probe.Map("exec_file_cache"); err != nil {
		return err
	}

	if p.procCacheMap, err = p.probe.Map("proc_cache"); err != nil {
		return err
	}

	if p.pidCacheMap, err = p.probe.Map("pid_cache"); err != nil {
		return err
	}

	go p.cacheFlush(ctx)

	return nil
}

func (p *ProcessResolver) cacheFlush(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			procPids, err := process.Pids()
			if err != nil {
				continue
			}
			procPidsMap := make(map[uint32]bool)
			for _, pid := range procPids {
				procPidsMap[uint32(pid)] = true
			}

			p.Lock()
			for pid := range p.entryCache {
				if _, exists := procPidsMap[pid]; !exists {
					if entry := p.entryCache[pid]; entry != nil {
						p.exitedQueue = append(p.exitedQueue, pid)
					}
				}
			}
			p.Unlock()
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

	filledProc := utils.GetFilledProcess(proc)
	if filledProc == nil {
		return false
	}

	_, ret := p.syncCache(proc, filledProc)
	return ret
}

func (p *ProcessResolver) setAncestor(pce *model.ProcessCacheEntry) {
	parent := p.entryCache[pce.PPid]
	if parent != nil {
		pce.SetAncestor(parent)
	}
}

// syncCache snapshots /proc for the provided pid. This method returns true if it updated the process cache.
func (p *ProcessResolver) syncCache(proc *process.Process, filledProc *process.FilledProcess) (*model.ProcessCacheEntry, bool) {
	pid := uint32(proc.Pid)

	// Check if an entry is already in cache for the given pid.
	entry := p.entryCache[pid]
	if entry != nil {
		p.setAncestor(entry)

		return entry, false
	}

	entry = p.NewProcessCacheEntry(model.PIDContext{Pid: pid, Tid: pid})

	// update the cache entry
	if err := p.enrichEventFromProc(entry, proc, filledProc); err != nil {
		entry.Release()

		seclog.Trace(err)
		return nil, false
	}

	p.setAncestor(entry)

	p.insertEntry(entry, p.entryCache[pid])

	// insert new entry in kernel maps
	procCacheEntryB := make([]byte, 224)
	_, err := entry.Process.MarshalProcCache(procCacheEntryB)
	if err != nil {
		seclog.Errorf("couldn't marshal proc_cache entry: %s", err)
	} else {
		if err = p.procCacheMap.Put(entry.Cookie, procCacheEntryB); err != nil {
			seclog.Errorf("couldn't push proc_cache entry to kernel space: %s", err)
		}
	}
	pidCacheEntryB := make([]byte, 64)
	_, err = entry.Process.MarshalPidCache(pidCacheEntryB)
	if err != nil {
		seclog.Errorf("couldn't marshal prid_cache entry: %s", err)
	} else {
		if err = p.pidCacheMap.Put(pid, pidCacheEntryB); err != nil {
			seclog.Errorf("couldn't push pid_cache entry to kernel space: %s", err)
		}
	}

	seclog.Tracef("New process cache entry added: %s %s %d/%d", entry.Comm, entry.FileEvent.PathnameStr, pid, entry.FileEvent.Inode)

	return entry, true
}

func (p *ProcessResolver) dumpEntry(writer io.Writer, entry *model.ProcessCacheEntry, already map[string]bool, withArgs bool) {
	for entry != nil {
		label := fmt.Sprintf("%s:%d", entry.Comm, entry.Pid)
		if _, exists := already[label]; !exists {
			if !entry.ExitTime.IsZero() {
				label = "[" + label + "]"
			}

			if withArgs {
				argv, _ := p.GetProcessScrubbedArgv(&entry.Process)
				fmt.Fprintf(writer, `"%d:%s" [label="%s", comment="%s"];`, entry.Pid, entry.Comm, label, strings.Join(argv, " "))
			} else {
				fmt.Fprintf(writer, `"%d:%s" [label="%s"];`, entry.Pid, entry.Comm, label)
			}
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
func (p *ProcessResolver) Dump(withArgs bool) (string, error) {
	dump, err := os.CreateTemp("/tmp", "process-cache-dump-")
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
		p.dumpEntry(dump, entry, already, withArgs)
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
	return float64(p.cacheSize.Load())
}

// SetState sets the process resolver state
func (p *ProcessResolver) SetState(state int64) {
	p.state.Store(state)
}

// Walk iterates through the entire tree and call the provided callback on each entry
func (p *ProcessResolver) Walk(callback func(entry *model.ProcessCacheEntry)) {
	p.RLock()
	defer p.RUnlock()

	for _, entry := range p.entryCache {
		callback(entry)
	}
}

// NewProcessVariables returns a provider for variables attached to a process cache entry
func (p *ProcessResolver) NewProcessVariables(scoper func(ctx *eval.Context) unsafe.Pointer) rules.VariableProvider {
	var variables *eval.ScopedVariables
	variables = eval.NewScopedVariables(scoper, func(key unsafe.Pointer) {
		(*model.ProcessCacheEntry)(key).SetReleaseCallback(func() {
			variables.ReleaseVariable(key)
		})
	})

	return variables
}

// NewProcessResolver returns a new process resolver
func NewProcessResolver(probe *Probe, resolvers *Resolvers, opts ProcessResolverOpts) (*ProcessResolver, error) {
	argsEnvsCache, err := simplelru.NewLRU(maxParallelArgsEnvs, nil)
	if err != nil {
		return nil, err
	}

	p := &ProcessResolver{
		probe:          probe,
		resolvers:      resolvers,
		entryCache:     make(map[uint32]*model.ProcessCacheEntry),
		opts:           opts,
		argsEnvsCache:  argsEnvsCache,
		state:          atomic.NewInt64(snapshotting),
		argsEnvsPool:   NewArgsEnvsPool(maxArgsEnvResidents),
		hitsStats:      map[string]*atomic.Int64{},
		cacheSize:      atomic.NewInt64(0),
		missStats:      atomic.NewInt64(0),
		addedEntries:   atomic.NewInt64(0),
		flushedEntries: atomic.NewInt64(0),
		pathErrStats:   atomic.NewInt64(0),
		argsTruncated:  atomic.NewInt64(0),
		argsSize:       atomic.NewInt64(0),
		envsTruncated:  atomic.NewInt64(0),
		envsSize:       atomic.NewInt64(0),
	}
	for _, t := range metrics.AllTypesTags {
		p.hitsStats[t] = atomic.NewInt64(0)
	}
	p.processCacheEntryPool = NewProcessCacheEntryPool(p)

	return p, nil
}

// NewProcessResolverOpts returns a new set of process resolver options
func NewProcessResolverOpts(envsWithValue []string) ProcessResolverOpts {
	opts := ProcessResolverOpts{
		envsWithValue: make(map[string]bool, len(envsWithValue)),
	}

	for _, envVar := range envsWithValue {
		opts.envsWithValue[envVar] = true
	}

	return opts
}
