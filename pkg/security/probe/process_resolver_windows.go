// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows
// +build windows

package probe

import (
	"strings"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-go/v5/statsd"
)

type Pid uint32
type ProcessEntry struct {
	Pid         Pid
	ImageFile   string
	CommandLine []string
}
type ProcessResolver struct {
	maplock   sync.Mutex
	processes map[Pid]*ProcessEntry
	opts      ProcessResolverOpts
}

// ProcessResolverOpts options of resolver
type ProcessResolverOpts struct {
}

// NewProcessResolver returns a new process resolver
func NewProcessResolver(config *config.Config, statsdClient statsd.ClientInterface,
	opts ProcessResolverOpts) (*ProcessResolver, error) {

	p := &ProcessResolver{
		processes: make(map[Pid]*ProcessEntry),
		opts:      opts,
	}

	return p, nil
}

// NewProcessResolverOpts returns a new set of process resolver options
func NewProcessResolverOpts() ProcessResolverOpts {
	return ProcessResolverOpts{}
}

func (p *ProcessResolver) AddNewProcessEntry(pid Pid, file string, commandLine string) (*ProcessEntry, error) {
	e := &ProcessEntry{
		Pid:         pid,
		ImageFile:   file,
		CommandLine: strings.Split(commandLine, " "),
	}
	p.maplock.Lock()
	defer p.maplock.Unlock()
	p.processes[pid] = e
	return e, nil
}

func (p *ProcessResolver) GetProcessEntry(pid Pid) *ProcessEntry {
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
