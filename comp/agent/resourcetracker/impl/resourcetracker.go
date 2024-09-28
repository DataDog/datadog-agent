// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package resourcetrackerimpl implements the resourcetracker component interface
package resourcetrackerimpl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/shirou/gopsutil/v4/process"

	resourcetracker "github.com/DataDog/datadog-agent/comp/agent/resourcetracker/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// Requires defines the dependencies for the resourcetracker component
type Requires struct {
	Lifecycle compdef.Lifecycle
	Log       log.Component
	Submitter resourcetracker.Submitter
}

// Provides defines the output of the resourcetracker component
type Provides struct {
	Comp resourcetracker.Component
}

// NewComponent creates a new resourcetracker component
func NewComponent(reqs Requires) (Provides, error) {
	tracker := resourceTracker{
		stop:      make(chan struct{}),
		log:       reqs.Log,
		submitter: reqs.Submitter,
	}
	reqs.Lifecycle.Append(compdef.Hook{
		OnStop:  tracker.Stop,
		OnStart: tracker.Start,
	})
	return Provides{Comp: tracker}, nil
}

type resourceTracker struct {
	stop      chan struct{}
	log       log.Component
	submitter resourcetracker.Submitter
}

func (t *resourceTracker) Start(_ context.Context) error {
	go t.run()
	return nil
}

// Stop stops the resource tracker
func (t *resourceTracker) Stop(_ context.Context) error {
	close(t.stop)
	return nil
}

func (r *resourceTracker) run() {
	r.submitResourceUsage()
	ticker := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-ticker.C:
			r.submitResourceUsage()
		case <-r.stop:
			return
		}
	}
}

func (r *resourceTracker) submitResourceUsage() {
	proc, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		r.log.Debugf("failed to get process: %v", err)
		return
	}
	exe, err := proc.Exe()
	if err != nil {
		r.log.Debugf("failed to get exe: %v", err)
		return
	}
	exe = filepath.Base(exe)
	rss, err := getRSS(proc)
	if err != nil {
		r.log.Debugf("failed to get rss: %v", err)
		return
	}
	cpu, err := getCPU(proc)
	if err != nil {
		r.log.Debugf("failed to get cpu: %v", err)
		return
	}
	tags := []string{
		fmt.Sprintf("pid:%d", proc.Pid),
		fmt.Sprintf("process:%s", exe),
	}
	r.submitter.Gauge("datadog.agent.process.cpu", cpu, tags)
	r.submitter.Gauge("datadog.agent.process.rss", float64(rss), tags)
}

func getRSS(proc *process.Process) (uint64, error) {
	mem, err := proc.MemoryInfo()
	if err != nil {
		return 0, err
	}
	return mem.RSS, nil
}

func getCPU(proc *process.Process) (float64, error) {
	cpu, err := proc.CPUPercent()
	if err != nil {
		return 0, err
	}
	return cpu, nil
}
