// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package process holds process related files
package process

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Pid PID type
type Pid = uint32

// Resolver defines a resolver
type Resolver struct {
	sync.RWMutex
	processes map[Pid]*model.ProcessCacheEntry
	scrubber  *procutil.DataScrubber

	processCacheEntryPool *Pool
}

// NewResolver returns a new process resolver
func NewResolver(_ *config.Config, scrubber *procutil.DataScrubber) (*Resolver, error) {
	p := &Resolver{
		processes: make(map[Pid]*model.ProcessCacheEntry),
		scrubber:  scrubber,
	}

	p.processCacheEntryPool = NewProcessCacheEntryPool(func() {})

	return p, nil
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
	} else {
		log.Tracef("unable to find parent of %v\n", entry)
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
func (p *Resolver) AddNewEntry(pid uint32, ppid uint32, file string, args []string) (*model.ProcessCacheEntry, error) {
	e := p.processCacheEntryPool.Get()
	e.PIDContext.Pid = pid
	e.PPid = ppid

	e.Process.Argv = args
	e.Process.FileEvent.PathnameStr = file
	e.Process.FileEvent.BasenameStr = filepath.Base(e.Process.FileEvent.PathnameStr)
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
func (p *Resolver) Resolve(pid uint32) *model.ProcessCacheEntry { //nolint:revive // TODO fix revive unused-parameter
	return p.GetEntry(pid)
}

// GetEnvs returns the envs of the event
func (p *Resolver) GetEnvs(pr *model.Process) []string {
	if pr.EnvsEntry == nil {
		return pr.Envs
	}

	keys, _ := pr.EnvsEntry.FilterEnvs(nil)
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

// GetProcessCmdLineScrubbed returns the scrubbed cmdline
func (p *Resolver) GetProcessCmdLineScrubbed(pr *model.Process) string {
	if pr.ScrubbedCmdLineResolved {
		return pr.CmdLine
	}

	if p.scrubber != nil && len(pr.CmdLine) > 0 {
		// replace with the scrubbed version
		scrubbed, _ := p.scrubber.ScrubCommand([]string{pr.CmdLine})
		if len(scrubbed) > 0 {
			pr.CmdLine = strings.Join(scrubbed, " ")
		}
	}

	return pr.CmdLine
}

// SendStats sends process resolver metrics
func (p *Resolver) SendStats() error {
	return nil
}

// Snapshot snapshot existing processes
func (p *Resolver) Snapshot() {
	processes, err := utils.GetProcesses()
	if err != nil {
		log.Errorf("failed to list processes: %v", err)
		return
	}

	entries := make([]*model.ProcessCacheEntry, 0, len(processes))

	for _, proc := range processes {
		fp, err := utils.GetFilledProcess(proc)
		if err != nil {
			log.Errorf("failed to fill process cache: %v", err)
			continue
		}

		execPath, err := proc.Exe()
		if err != nil {
			log.Errorf("failed to fetch exec path: %v", err)
			continue
		}

		pCreateTime, err := proc.CreateTime()
		if err != nil {
			log.Errorf("failted to fetch create time: %v", err)
		}

		e := p.processCacheEntryPool.Get()
		e.PIDContext.Pid = Pid(fp.Pid)
		e.PPid = Pid(fp.Ppid)

		e.Process.Argv = fp.Cmdline
		e.Process.FileEvent.PathnameStr = execPath
		e.Process.FileEvent.BasenameStr = filepath.Base(execPath)
		e.ExecTime = time.Unix(0, pCreateTime*int64(time.Millisecond))

		entries = append(entries, e)
	}

	// make sure to insert them in the creation time order
	sort.Slice(entries, func(i, j int) bool {
		entryA := entries[i]
		entryB := entries[j]

		if entryA.ExecTime.Equal(entryB.ExecTime) {
			return entries[i].Pid < entries[j].Pid
		}

		return entryA.ExecTime.Before(entryB.ExecTime)
	})

	for _, e := range entries {
		p.insertEntry(e)
	}
}
