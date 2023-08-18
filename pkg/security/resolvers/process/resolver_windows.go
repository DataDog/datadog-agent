// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package process

import (
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-go/v5/statsd"
)

type Pid = uint32

type Resolver struct {
	maplock   sync.Mutex
	processes map[Pid]*model.ProcessCacheEntry
	opts      ResolverOpts
}

// ResolverOpts options of resolver
type ResolverOpts struct {
}

// NewResolver returns a new process resolver
func NewResolver(config *config.Config, statsdClient statsd.ClientInterface,
	opts ResolverOpts) (*Resolver, error) {

	p := &Resolver{
		processes: make(map[Pid]*model.ProcessCacheEntry),
		opts:      opts,
	}

	return p, nil
}

// NewResolverOpts returns a new set of process resolver options
func NewResolverOpts() ResolverOpts {
	return ResolverOpts{}
}

func (p *Resolver) AddNewProcessEntry(pid Pid, file string, commandLine string) (*model.ProcessCacheEntry, error) {
	e := model.NewProcessCacheEntry(nil)

	e.Process.PIDContext.Pid = uint32(e.Pid)
	e.Process.Argv0 = file
	e.Process.Argv = strings.Split(commandLine, " ")

	// where do we put the file and the command line?
	p.maplock.Lock()
	defer p.maplock.Unlock()
	p.processes[pid] = e
	return e, nil
}

func (p *Resolver) GetProcessEntry(pid Pid) *model.ProcessCacheEntry {
	p.maplock.Lock()
	defer p.maplock.Unlock()
	if e, ok := p.processes[pid]; ok {
		return e
	}
	return nil
}

func (p *Resolver) DeleteProcessEntry(pid Pid) {
	p.maplock.Lock()
	defer p.maplock.Unlock()
	if _, ok := p.processes[pid]; ok {
		delete(p.processes, pid)
	}
}

// Resolve returns the cache entry for the given pid
func (p *Resolver) Resolve(pid, tid uint32, inode uint64, useFallBack bool) *model.ProcessCacheEntry {
	return p.GetProcessEntry(pid)
}
