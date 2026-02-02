// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probe holds probe related files
package probe

import (
	"os"
	"slices"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/hash"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

const (
	defaultHashActionFlushDelay = 5 * time.Second
	fileSizeCheckInterval       = 1 * time.Second
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
	p.resolver.HashFileEvent(report.eventType, report.crtID, report.pid, &report.fileEvent)
	report.resolved = true
}

// checkFileSizeStable checks if the file size has stabilized between stat calls
func (p *FileHasher) checkFileSizeStable(report *HashActionReport) bool {
	now := time.Now()

	// Check if it's time to stat the file again (every 1 second)
	if now.Before(report.lastStatAt.Add(fileSizeCheckInterval)) {
		return false
	}

	// Stat the file to get current size
	fileInfo, err := os.Stat(report.fileEvent.PathnameStr)
	if err != nil {
		seclog.Debugf("failed to stat file %s: %v", report.fileEvent.PathnameStr, err)
		return false
	}

	currentSize := fileInfo.Size()
	report.lastStatAt = now

	// First stat - initialize the size
	if report.lastFileSize == 0 {
		report.lastFileSize = currentSize
		return false
	}

	// Check if size changed
	if currentSize != report.lastFileSize {
		report.lastFileSize = currentSize
		return false
	}

	// Size is stable (unchanged between two iterations)
	return true
}

// FlushPendingReports flush pending reports
func (p *FileHasher) FlushPendingReports() {
	p.Lock()
	defer p.Unlock()

	p.pendingReports = slices.DeleteFunc(p.pendingReports, func(report *HashActionReport) bool {
		report.Lock()
		defer report.Unlock()

		// Check if file size has stabilized
		if p.checkFileSizeStable(report) {
			report.Trigger = HashTriggerFileSizeStable
			p.hash(report)
			return true
		}

		// Fall back to timeout-based flush
		if time.Now().After(report.firstSeenAt.Add(defaultHashActionFlushDelay)) {
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
func (p *FileHasher) HashAndReport(rule *rules.Rule, action *rules.HashDefinition, ev *model.Event) bool {
	eventType := ev.GetEventType()

	if !p.cfg.RuntimeSecurity.HashResolverEnabled {
		return false
	}

	fileEvent, err := ev.GetFileField(action.Field)
	if err != nil {
		seclog.Errorf("failed to get file field %s: %v", action.Field, err)
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
		crtID:       ev.ProcessContext.Process.ContainerContext.ContainerID,
		firstSeenAt: ev.ResolveEventTime(),
		fileEvent:   *fileEvent,
		eventType:   eventType,
	}
	ev.ActionReports = append(ev.ActionReports, report)
	p.pendingReports = append(p.pendingReports, report)

	return true
}
