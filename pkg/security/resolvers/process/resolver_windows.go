// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package process holds process related files
package process

import (
	"fmt"
	"path"
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// Pid PID type
type Pid = uint32

// Resolver defines a resolver
type Resolver struct {
	sync.RWMutex
	processes    map[Pid]*model.ProcessCacheEntry
	opts         ResolverOpts
	scrubber     *procutil.DataScrubber
	statsdClient statsd.ClientInterface

	// stats
	cacheSize *atomic.Int64

	processCacheEntryPool *Pool
}

// ResolverOpts options of resolver
type ResolverOpts struct {
	envsWithValue map[string]bool
}

// NewResolver returns a new process resolver
func NewResolver(config *config.Config, statsdClient statsd.ClientInterface, scrubber *procutil.DataScrubber,
	opts ResolverOpts) (*Resolver, error) {

	p := &Resolver{
		processes:    make(map[Pid]*model.ProcessCacheEntry),
		opts:         opts,
		scrubber:     scrubber,
		cacheSize:    atomic.NewInt64(0),
		statsdClient: statsdClient,
	}

	p.processCacheEntryPool = NewProcessCacheEntryPool(p)

	return p, nil
}

// NewResolverOpts returns a new set of process resolver options
func NewResolverOpts() ResolverOpts {
	return ResolverOpts{}
}

func (p *Resolver) insertEntry(entry *model.ProcessCacheEntry) {
	// PID collision
	if prev := p.processes[entry.Pid]; prev != nil {
		prev.Release()
	}

	p.processes[entry.Pid] = entry
	entry.Retain()

	parent := p.processes[entry.PPid]
	if parent != nil {
		entry.SetAncestor(parent)
	}
}

func (p *Resolver) deleteEntry(pid uint32, exitTime time.Time) {
	entry, ok := p.processes[pid]
	if !ok {
		return
	}

	entry.Exit(exitTime)
	delete(p.processes, entry.Pid)
	entry.Release()
}

// DeleteEntry tries to delete an entry in the process cache
func (p *Resolver) DeleteEntry(pid uint32, exitTime time.Time) {
	p.Lock()
	defer p.Unlock()

	p.deleteEntry(pid, exitTime)
}

// AddNewEntry add a new process entry to the cache
func (p *Resolver) AddNewEntry(pid uint32, ppid uint32, file string, commandLine string) (*model.ProcessCacheEntry, error) {
	e := p.processCacheEntryPool.Get()
	e.PIDContext.Pid = pid
	e.PPid = ppid

	e.Process.Args = commandLine
	e.Process.FileEvent.PathnameStr = file
	e.Process.FileEvent.BasenameStr = path.Base(e.Process.FileEvent.PathnameStr)
	e.ExecTime = time.Now()

	p.insertEntry(e)

	return e, nil
}

// GetEntry returns the process entry for the given pid
func (p *Resolver) GetEntry(pid Pid) *model.ProcessCacheEntry {
	p.Lock()
	defer p.Unlock()
	if e, ok := p.processes[pid]; ok {
		return e
	}
	return nil
}

// Resolve returns the cache entry for the given pid
func (p *Resolver) Resolve(pid, tid uint32, inode uint64, useFallBack bool) *model.ProcessCacheEntry {
	return p.GetEntry(pid)
}

// GetEnvs returns the envs of the event
func (p *Resolver) GetEnvs(pr *model.Process) []string {
	if pr.EnvsEntry == nil {
		return pr.Envs
	}

	keys, _ := pr.EnvsEntry.FilterEnvs(p.opts.envsWithValue)
	pr.Envs = keys
	return pr.Envs
}

// GetEnvp returns the envs of the event with their values
func (p *Resolver) GetEnvp(pr *model.Process) []string {
	if pr.EnvsEntry == nil {
		return pr.Envp
	}

	pr.Envp = pr.EnvsEntry.Values
	return pr.Envp
}

// getCacheSize returns the cache size of the process resolver
func (p *Resolver) getCacheSize() float64 {
	p.RLock()
	defer p.RUnlock()
	return float64(len(p.processes))
}

// SendStats sends process resolver metrics
func (p *Resolver) SendStats() error {
	if err := p.statsdClient.Gauge(metrics.MetricProcessResolverCacheSize, p.getCacheSize(), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send process_resolver cache_size metric: %w", err)
	}

	return nil
}

// Snapshot snapshot existing processes
func (p *Resolver) Snapshot() {
	puprobe := procutil.NewWindowsToolhelpProbe()
	pmap, err := puprobe.ProcessesByPID(time.Now(), false)
	if err != nil {
		return
	}
	// the list returned is a map of pid to procutil.Process.
	// The processes can be iterated with the following caveats
	// Pid should be valid
	// Ppid should be valid (with more caveats below)
	// The `exe` field is the unqualified name of the executable (no path)
	// the `Cmdline` is an array of strings, parsed on ` ` boundaries
	// the `stats` field is mostly not filled in because of the `false` argument to `ProcessesByPID()`
	//     however, the create time will be filled in
	for pid, proc := range pmap {
		e := p.processCacheEntryPool.Get()
		e.PIDContext.Pid = Pid(pid)
		e.PPid = Pid(proc.Ppid)

		e.Process.Args = strings.Join(proc.GetCmdline(), " ")
		e.Process.FileEvent.PathnameStr = proc.Exe
		e.Process.FileEvent.BasenameStr = path.Base(e.Process.FileEvent.PathnameStr)
		e.ExecTime = time.Now()

		p.insertEntry(e)

		log.Tracef("PID %d  %d PPID %d\n", pid, proc.Pid, proc.Ppid)
		log.Tracef("  executable %s\n", proc.Exe)
		log.Tracef("  executable %v\n", proc.GetCmdline())
		log.Tracef("  createtime %v\n", proc.Stats.CreateTime)
	}
	// another note on PPids.  Windows reuses process IDS.  So consider the following

	// process 1 starts
	// process 1 starts process 2 (so 1 is the parent of 2)
	// process 1 ends/dies
	// another process starts and is given the pid (1)
	// process 2's PPid will still be 2, but the current Pid(1) was not the one that created pid 2.
}
