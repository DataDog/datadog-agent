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

type Pid uint32
type ProcessResolver struct {
	maplock   sync.Mutex
	processes map[Pid]*model.ProcessCacheEntry
	opts      ProcessResolverOpts
}

// ProcessResolverOpts options of resolver
type ProcessResolverOpts struct {
}

// NewProcessResolver returns a new process resolver
func NewResolver(config *config.Config, statsdClient statsd.ClientInterface,
	opts ProcessResolverOpts) (*ProcessResolver, error) {

	p := &ProcessResolver{
		processes: make(map[Pid]*model.ProcessCacheEntry),
		opts:      opts,
	}

	return p, nil
}

// NewProcessResolverOpts returns a new set of process resolver options
func NewResolverOpts() ProcessResolverOpts {
	return ProcessResolverOpts{}
}

func (p *ProcessResolver) AddNewProcessEntry(pid Pid, file string, commandLine string) (*model.ProcessCacheEntry, error) {
	e := model.NewEmptyProcessCacheEntry(uint32(pid), 0, false)

	e.Process.PIDContext.Pid = uint32(e.Pid)
	e.Process.Argv0 = file
	e.Process.Argv = strings.Split(commandLine, " ")

	// where do we put the file and the command line?
	p.maplock.Lock()
	defer p.maplock.Unlock()
	p.processes[pid] = e
	return e, nil
}

func (p *ProcessResolver) GetProcessEntry(pid Pid) *model.ProcessCacheEntry {
	p.maplock.Lock()
	defer p.maplock.Unlock()
	if e, ok := p.processes[pid]; ok {
		return e
	}
	return nil
}

func (p *ProcessResolver) DeleteProcessEntry(pid Pid) {
	p.maplock.Lock()
	defer p.maplock.Unlock()
	if _, ok := p.processes[pid]; ok {
		delete(p.processes, pid)
	}
}
