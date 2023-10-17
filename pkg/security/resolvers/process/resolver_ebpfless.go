// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ebpfless

// Package process holds process related files
package process

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Resolver defines a resolver
type Resolver struct {
	sync.RWMutex
	entryCache   map[uint32]*model.ProcessCacheEntry
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

// WithEnvsValue specifies envs with value
func (o *ResolverOpts) WithEnvsValue(envsWithValue []string) *ResolverOpts {
	for _, envVar := range envsWithValue {
		o.envsWithValue[envVar] = true
	}
	return o
}

// NewResolver returns a new process resolver
func NewResolver(config *config.Config, statsdClient statsd.ClientInterface, scrubber *procutil.DataScrubber,
	opts ResolverOpts) (*Resolver, error) {

	p := &Resolver{
		entryCache:   make(map[uint32]*model.ProcessCacheEntry),
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

func (p *Resolver) deleteEntry(pid uint32, exitTime time.Time) {
	entry, ok := p.entryCache[pid]
	if !ok {
		return
	}

	entry.Exit(exitTime)
	delete(p.entryCache, entry.Pid)
	entry.Release()
}

// DeleteEntry tries to delete an entry in the process cache
func (p *Resolver) DeleteEntry(pid uint32, exitTime time.Time) {
	p.Lock()
	defer p.Unlock()

	p.deleteEntry(pid, exitTime)
}

// AddForkEntry adds an entry to the local cache and returns the newly created entry
func (p *Resolver) AddForkEntry(pid uint32, ppid uint32) *model.ProcessCacheEntry {
	entry := p.processCacheEntryPool.Get()
	entry.PIDContext.Pid = pid
	entry.PPid = ppid

	p.Lock()
	defer p.Unlock()

	p.insertForkEntry(entry, model.ProcessCacheEntryFromEvent)

	return entry
}

// AddExecEntry adds an entry to the local cache and returns the newly created entry
func (p *Resolver) AddExecEntry(pid uint32, file string, argv []string, envs []string) *model.ProcessCacheEntry {
	entry := p.processCacheEntryPool.Get()
	entry.PIDContext.Pid = pid

	entry.Process.Argv = argv
	if len(argv) > 0 {
		entry.Process.Argv0 = argv[0]
	}
	entry.Process.Envs = envs
	entry.Process.FileEvent.PathnameStr = file
	entry.Process.FileEvent.BasenameStr = filepath.Base(entry.Process.FileEvent.PathnameStr)
	entry.ExecTime = time.Now()

	p.Lock()
	defer p.Unlock()

	p.insertExecEntry(entry, model.ProcessCacheEntryFromEvent)

	return entry
}

func (p *Resolver) insertEntry(entry, prev *model.ProcessCacheEntry, source uint64) {
	entry.Source = source
	p.entryCache[entry.Pid] = entry
	entry.Retain()

	if prev != nil {
		prev.Release()
	}

	p.cacheSize.Inc()
}

func (p *Resolver) insertForkEntry(entry *model.ProcessCacheEntry, source uint64) {
	prev := p.entryCache[entry.Pid]
	if prev != nil {
		// this shouldn't happen but it is better to exit the prev and let the new one replace it
		prev.Exit(entry.ForkTime)
	}

	if entry.Pid != 1 {
		parent := p.entryCache[entry.PPid]
		if parent != nil {
			parent.Fork(entry)
		} else {
			entry.IsParentMissing = true
		}
	}

	p.insertEntry(entry, prev, source)
}

func (p *Resolver) insertExecEntry(entry *model.ProcessCacheEntry, source uint64) {
	prev := p.entryCache[entry.Pid]
	if prev != nil {
		prev.Exec(entry)
	}

	p.insertEntry(entry, prev, source)
}

// Resolve returns the cache entry for the given pid
func (p *Resolver) Resolve(pid uint32) *model.ProcessCacheEntry {
	p.Lock()
	defer p.Unlock()
	if e, ok := p.entryCache[pid]; ok {
		return e
	}
	return nil
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
	return float64(len(p.entryCache))
}

// SendStats sends process resolver metrics
func (p *Resolver) SendStats() error {
	if err := p.statsdClient.Gauge(metrics.MetricProcessResolverCacheSize, p.getCacheSize(), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send process_resolver cache_size metric: %w", err)
	}

	return nil
}

// GetProcessArgvScrubbed returns the scrubbed args of the event as an array
func (p *Resolver) GetProcessArgvScrubbed(pr *model.Process) ([]string, bool) {
	argv, _ := p.scrubber.ScrubCommand(pr.Argv)
	return argv, false
}

// GetProcessEnvs returns the envs of the event
func (p *Resolver) GetProcessEnvs(pr *model.Process) ([]string, bool) {
	return pr.Envs, false
}

// GetProcessArgv0 returns the first arg of the event and whether the process arguments are truncated
func GetProcessArgv0(pr *model.Process) (string, bool) {
	return pr.Argv0, false
}

// Start starts the resolver
func (p *Resolver) Start(ctx context.Context) error {
	return nil
}

// Snapshot snapshot existing entryCache
func (p *Resolver) Snapshot() {}
