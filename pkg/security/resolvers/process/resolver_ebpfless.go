// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package process holds process related files
package process

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// CacheResolverKey is used to store and retrieve processes from the cache
type CacheResolverKey struct {
	Pid  uint32 // Pid of the related process (namespaced)
	NSID uint64 // NSID represents the pids namespace ID of the related container
}

// EBPFLessResolver defines a resolver
type EBPFLessResolver struct {
	sync.RWMutex
	entryCache   map[CacheResolverKey]*model.ProcessCacheEntry
	opts         ResolverOpts
	scrubber     *procutil.DataScrubber
	statsdClient statsd.ClientInterface

	// stats
	cacheSize *atomic.Int64

	processCacheEntryPool *Pool
}

// NewEBPFLessResolver returns a new process resolver
func NewEBPFLessResolver(_ *config.Config, statsdClient statsd.ClientInterface, scrubber *procutil.DataScrubber, opts *ResolverOpts) (*EBPFLessResolver, error) {
	p := &EBPFLessResolver{
		entryCache:   make(map[CacheResolverKey]*model.ProcessCacheEntry),
		opts:         *opts,
		scrubber:     scrubber,
		cacheSize:    atomic.NewInt64(0),
		statsdClient: statsdClient,
	}

	p.processCacheEntryPool = NewProcessCacheEntryPool(func() { p.cacheSize.Dec() })

	return p, nil
}

func (p *EBPFLessResolver) deleteEntry(key CacheResolverKey, exitTime time.Time) {
	entry, ok := p.entryCache[key]
	if !ok {
		return
	}

	entry.Exit(exitTime)
	delete(p.entryCache, key)
	entry.Release()
}

// DeleteEntry tries to delete an entry in the process cache
func (p *EBPFLessResolver) DeleteEntry(key CacheResolverKey, exitTime time.Time) {
	p.Lock()
	defer p.Unlock()

	p.deleteEntry(key, exitTime)
}

// AddForkEntry adds an entry to the local cache and returns the newly created entry
func (p *EBPFLessResolver) AddForkEntry(key CacheResolverKey, ppid uint32, ts uint64) *model.ProcessCacheEntry {
	if key.Pid == 0 {
		return nil
	}

	entry := p.processCacheEntryPool.Get()
	entry.PIDContext.Pid = key.Pid
	entry.PPid = ppid
	entry.ForkTime = time.Unix(0, int64(ts))
	entry.Source = model.ProcessCacheEntryFromEvent

	p.Lock()
	defer p.Unlock()

	p.insertForkEntry(key, entry)

	return entry
}

// NewEntry returns a new entry
func (p *EBPFLessResolver) NewEntry(key CacheResolverKey, ppid uint32, file string, argv []string, argsTruncated bool,
	envs []string, envsTruncated bool, ctrID string, ts uint64, tty string, source uint64) *model.ProcessCacheEntry {

	entry := p.processCacheEntryPool.Get()
	entry.PIDContext.Pid = key.Pid
	entry.PPid = ppid
	entry.Source = source

	entry.Process.ArgsEntry = &model.ArgsEntry{
		Values:    argv,
		Truncated: argsTruncated,
	}
	if len(argv) > 0 {
		entry.Process.Argv0 = argv[0]
	}
	entry.Process.Comm = filepath.Base(file)
	if len(entry.Process.Comm) > 16 {
		// truncate comm to max 16 chars to be ebpf ISO
		entry.Process.Comm = entry.Process.Comm[:16]
	}
	entry.Process.TTYName = tty

	entry.Process.EnvsEntry = &model.EnvsEntry{
		Values:    envs,
		Truncated: envsTruncated,
	}

	if strings.HasPrefix(file, "memfd:") {
		entry.Process.FileEvent.PathnameStr = ""
		entry.Process.FileEvent.BasenameStr = file
	} else {
		entry.Process.FileEvent.PathnameStr = file
		entry.Process.FileEvent.BasenameStr = filepath.Base(entry.Process.FileEvent.PathnameStr)
	}
	entry.Process.ContainerID = containerutils.ContainerID(ctrID)

	entry.ExecTime = time.Unix(0, int64(ts))

	return entry
}

// AddExecEntry adds an entry to the local cache and returns the newly created entry
func (p *EBPFLessResolver) AddExecEntry(key CacheResolverKey, ppid uint32, file string, argv []string, argsTruncated bool,
	envs []string, envsTruncated bool, ctrID string, ts uint64, tty string) *model.ProcessCacheEntry {
	if key.Pid == 0 {
		return nil
	}

	entry := p.NewEntry(key, ppid, file, argv, argsTruncated, envs, envsTruncated, ctrID, ts, tty, model.ProcessCacheEntryFromEvent)

	p.Lock()
	defer p.Unlock()

	p.insertExecEntry(key, entry)

	return entry
}

// AddProcFSEntry add a procfs entry
func (p *EBPFLessResolver) AddProcFSEntry(key CacheResolverKey, ppid uint32, file string, argv []string, argsTruncated bool,
	envs []string, envsTruncated bool, ctrID string, ts uint64, tty string) *model.ProcessCacheEntry {
	if key.Pid == 0 {
		return nil
	}

	entry := p.NewEntry(key, ppid, file, argv, argsTruncated, envs, envsTruncated, ctrID, ts, tty, model.ProcessCacheEntryFromProcFS)

	p.Lock()
	defer p.Unlock()

	parentKey := CacheResolverKey{NSID: key.NSID, Pid: ppid}
	if parent := p.entryCache[parentKey]; parent != nil {
		if parent.Equals(entry) {
			entry.SetForkParent(parent)
		} else {
			entry.SetExecParent(parent)
		}
	}
	p.insertEntry(key, entry, p.entryCache[key])

	return entry
}

func (p *EBPFLessResolver) insertEntry(key CacheResolverKey, entry, prev *model.ProcessCacheEntry) {
	p.entryCache[key] = entry
	entry.Retain()

	if prev != nil {
		prev.Release()
	}

	p.cacheSize.Inc()
}

func (p *EBPFLessResolver) insertForkEntry(key CacheResolverKey, entry *model.ProcessCacheEntry) {
	if key.Pid == 0 {
		return
	}

	prev := p.entryCache[key]
	if prev != nil {
		// this shouldn't happen but it is better to exit the prev and let the new one replace it
		prev.Exit(entry.ForkTime)
	}

	if entry.Pid != 1 {
		parent := p.entryCache[CacheResolverKey{
			Pid:  entry.PPid,
			NSID: key.NSID,
		}]
		if parent != nil {
			parent.Fork(entry)
		}
	}

	p.insertEntry(key, entry, prev)
}

func (p *EBPFLessResolver) insertExecEntry(key CacheResolverKey, entry *model.ProcessCacheEntry) {
	if key.Pid == 0 {
		return
	}

	prev := p.entryCache[key]
	if prev != nil {
		prev.Exec(entry)

		// procfs entry should have the ppid already set
		if entry.PPid == 0 {
			entry.PPid = prev.PPid
		}

		// inheritate credentials as exec event, uid/gid can be update by setuid/setgid events
		entry.Credentials = prev.Credentials
	}

	p.insertEntry(key, entry, prev)
}

// Resolve returns the cache entry for the given pid
func (p *EBPFLessResolver) Resolve(key CacheResolverKey) *model.ProcessCacheEntry {
	if key.Pid == 0 {
		return nil
	}

	p.Lock()
	defer p.Unlock()
	if e, ok := p.entryCache[key]; ok {
		return e
	}
	return nil
}

// UpdateUID updates the credentials of the provided pid
func (p *EBPFLessResolver) UpdateUID(key CacheResolverKey, uid int32, euid int32) {
	p.Lock()
	defer p.Unlock()

	entry := p.entryCache[key]
	if entry != nil {
		if uid != -1 {
			entry.Credentials.UID = uint32(uid)
		}
		if euid != -1 {
			entry.Credentials.EUID = uint32(euid)
		}
	}
}

// UpdateGID updates the credentials of the provided pid
func (p *EBPFLessResolver) UpdateGID(key CacheResolverKey, gid int32, egid int32) {
	p.Lock()
	defer p.Unlock()

	entry := p.entryCache[key]
	if entry != nil {
		if gid != -1 {
			entry.Credentials.GID = uint32(gid)
		}
		if egid != -1 {
			entry.Credentials.EGID = uint32(egid)
		}
	}
}

// Walk iterates through the entire tree and call the provided callback on each entry
func (p *EBPFLessResolver) Walk(callback func(entry *model.ProcessCacheEntry)) {
	p.RLock()
	defer p.RUnlock()

	for _, entry := range p.entryCache {
		callback(entry)
	}
}

// getCacheSize returns the cache size of the process resolver
func (p *EBPFLessResolver) getCacheSize() float64 {
	p.RLock()
	defer p.RUnlock()
	return float64(len(p.entryCache))
}

// SendStats sends process resolver metrics
func (p *EBPFLessResolver) SendStats() error {
	if err := p.statsdClient.Gauge(metrics.MetricProcessResolverCacheSize, p.getCacheSize(), []string{}, 1.0); err != nil {
		return fmt.Errorf("failed to send process_resolver cache_size metric: %w", err)
	}

	return nil
}

// Start starts the resolver
func (p *EBPFLessResolver) Start(_ context.Context) error {
	return nil
}

// Snapshot snapshot existing entryCache
func (p *EBPFLessResolver) Snapshot() {}

// Dump create a temp file and dump the cache
func (p *EBPFLessResolver) Dump(_ bool) (string, error) {
	return "", errors.New("not supported")
}

// GetProcessArgvScrubbed returns the scrubbed args of the event as an array
func (p *EBPFLessResolver) GetProcessArgvScrubbed(pr *model.Process) ([]string, bool) {
	if pr.ArgsEntry == nil || pr.ScrubbedArgvResolved {
		return pr.Argv, pr.ArgsTruncated
	}

	if p.scrubber != nil && len(pr.ArgsEntry.Values) > 0 {
		// replace with the scrubbed version
		argv, _ := p.scrubber.ScrubCommand(pr.ArgsEntry.Values[1:])
		pr.ArgsEntry.Values = []string{pr.ArgsEntry.Values[0]}
		pr.ArgsEntry.Values = append(pr.ArgsEntry.Values, argv...)
	}
	pr.ScrubbedArgvResolved = true

	return GetProcessArgv(pr)
}

// GetProcessEnvs returns the envs of the event
func (p *EBPFLessResolver) GetProcessEnvs(pr *model.Process) ([]string, bool) {
	if pr.EnvsEntry == nil {
		return pr.Envs, pr.EnvsTruncated
	}

	keys, truncated := pr.EnvsEntry.FilterEnvs(p.opts.envsWithValue)
	pr.Envs = keys
	pr.EnvsTruncated = pr.EnvsTruncated || truncated
	return pr.Envs, pr.EnvsTruncated
}

// GetProcessEnvp returns the unscrubbed envs of the event with their values. Use with caution.
func (p *EBPFLessResolver) GetProcessEnvp(pr *model.Process) ([]string, bool) {
	if pr.EnvsEntry == nil {
		return pr.Envp, pr.EnvsTruncated
	}

	pr.Envp = pr.EnvsEntry.Values
	pr.EnvsTruncated = pr.EnvsTruncated || pr.EnvsEntry.Truncated
	return pr.Envp, pr.EnvsTruncated
}
