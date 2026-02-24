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

// FileHasher defines a file hasher structure
type FileHasher struct {
	sync.Mutex

	cfg      *config.Config
	resolver *hash.Resolver

	pendingReports []*HashActionReport
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
	p.resolver.HashFileEvent(report.eventType, report.cgroupID, report.pid, &report.fileEvent, report.maxFileSize)
	report.resolved = true
}

// FlushPendingReports flush pending reports
func (p *FileHasher) FlushPendingReports() {
	p.Lock()
	defer p.Unlock()

	p.pendingReports = slices.DeleteFunc(p.pendingReports, func(report *HashActionReport) bool {
		report.Lock()
		defer report.Unlock()

		hashDelay := defaultHashActionFlushDelay
		if report.eventType == model.ExecEventType {
			// Exec events can be hashed immediately since the executable file
			// is already on disk when we process the exec event
			hashDelay = 0
		}

		if time.Now().After(report.seenAt.Add(hashDelay)) {
			report.Trigger = HashTriggerTimeout
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
			report.Trigger = HashTriggerProcessExit
			p.hash(report)
			return true
		}
		return false
	})
}

// HashAndReport hash and report, returns true if the hash computation is supported for the given event
func (p *FileHasher) HashAndReport(rule *rules.Rule, action *rules.HashDefinition, ev *model.Event, fileEvent *model.FileEvent) bool {
	if !p.cfg.RuntimeSecurity.HashResolverEnabled {
		return false
	}

	if ev.ProcessContext.Pid == utils.Getpid() {
		return false
	}

	switch ev.Origin {
	case EBPFOrigin:
		if fileEvent.IsFileless() {
			return false
		}
	}

	report := &HashActionReport{
		rule:        rule,
		pid:         ev.ProcessContext.Pid,
		cgroupID:    ev.ProcessContext.Process.CGroup.CGroupID,
		maxFileSize: action.MaxFileSize,
		seenAt:      ev.ResolveEventTime(),
		fileEvent:   *fileEvent,
		eventType:   ev.GetEventType(),
	}
	ev.ActionReports = append(ev.ActionReports, report)
	p.pendingReports = append(p.pendingReports, report)

	return true
}
