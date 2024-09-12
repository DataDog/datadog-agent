// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"slices"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/hash"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	defaultHashActionFlushDelay = 5 * time.Second
)

// FileHasher defines a process killer structure
type FileHasher struct {
	sync.Mutex

	cfg      *config.Config
	resolver *hash.Resolver

	pendingReports []*HashActionReport
}

type FileHasherStats struct {
	actionPerformed int64
	processesKilled int64
}

// NewFileHasher returns a new FileHasher
func NewFileHasher(cfg *config.Config, resolver *hash.Resolver) *FileHasher {
	return &FileHasher{
		cfg:      cfg,
		resolver: resolver,
	}
}

// AddPendingReports add a pending reports
func (p *FileHasher) AddPendingReports(report *HashActionReport) {
	p.Lock()
	defer p.Unlock()

	p.pendingReports = append(p.pendingReports, report)
}

func (p *FileHasher) hash(report *HashActionReport) {
	p.resolver.HashFileEvent(report.eventType, report.crtID, report.pid, &report.fileEvent)
	report.resolved = true
}

// FlushPendingReports flush pending reports
func (p *FileHasher) FlushPendingReports() {
	p.Lock()
	defer p.Unlock()

	p.pendingReports = slices.DeleteFunc(p.pendingReports, func(report *HashActionReport) bool {
		report.Lock()
		defer report.Unlock()

		if time.Now().After(report.seenAt.Add(defaultHashActionFlushDelay)) {
			p.hash(report)
			return true
		}
		return false
	})
}

// HandleProcessExited handles process exited events
func (p *FileHasher) HandleProcessExited(event *model.Event) {
	p.Lock()
	defer p.Unlock()

	p.pendingReports = slices.DeleteFunc(p.pendingReports, func(report *HashActionReport) bool {
		report.Lock()
		defer report.Unlock()

		if report.pid == event.ProcessContext.Pid {
			p.hash(report)
			return true
		}
		return false
	})
}

// HashAndReport hash and report
func (p *FileHasher) HashAndReport(rule *rules.Rule, ev *model.Event) {
	eventType := ev.GetEventType()

	// only open events are supported
	if eventType != model.FileOpenEventType {
		return
	}

	if ev.ProcessContext.Pid == utils.Getpid() {
		return
	}

	report := &HashActionReport{
		rule:      rule,
		pid:       ev.ProcessContext.Pid,
		crtID:     ev.ProcessContext.ContainerID,
		seenAt:    ev.Timestamp,
		fileEvent: ev.Open.File,
		eventType: eventType,
	}
	ev.ActionReports = append(ev.ActionReports, report)
	p.pendingReports = append(p.pendingReports, report)
}
