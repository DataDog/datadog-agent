// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"sync/atomic"
	"time"
)

// ReadErrorReason classifies why a file could not be read
type ReadErrorReason int

const (
	// ReadErrorNotFound means no candidate path could be opened (process and file gone)
	ReadErrorNotFound ReadErrorReason = iota
	// ReadErrorPermission means the file exists but could not be opened
	ReadErrorPermission
	// ReadErrorOther covers every other open, stat or read failure
	ReadErrorOther

	readErrorReasonCount
)

// String returns the metric tag value of the reason
func (r ReadErrorReason) String() string {
	switch r {
	case ReadErrorNotFound:
		return "not_found"
	case ReadErrorPermission:
		return "permission"
	default:
		return "other"
	}
}

// Stats holds the pipeline counters. Every stage increments the counters it owns; the reporter
// reads them to emit metrics. All fields are safe for concurrent use.
type Stats struct {
	// consumer
	ExecsReceived atomic.Int64

	// dedupe and file access
	IdentityHits atomic.Int64
	ShaHits      atomic.Int64
	Reads        atomic.Int64
	ReadErrors   [readErrorReasonCount]atomic.Int64
	TooBig       atomic.Int64
	NotRegular   atomic.Int64

	// scan pool
	Scans        atomic.Int64
	Matches      atomic.Int64
	ScanErrors   atomic.Int64
	ScanTimeouts atomic.Int64
	QueueDrops   atomic.Int64

	// ObserveScanDuration, when set, is called after every scan. It must be set before the
	// pipeline starts and must be safe for concurrent use.
	ObserveScanDuration func(time.Duration)
}

// IncReadError increments the read error counter of reason
func (s *Stats) IncReadError(reason ReadErrorReason) {
	if reason < 0 || reason >= readErrorReasonCount {
		reason = ReadErrorOther
	}
	s.ReadErrors[reason].Add(1)
}

// ScanDone records the duration of a scan
func (s *Stats) ScanDone(d time.Duration) {
	if s.ObserveScanDuration != nil {
		s.ObserveScanDuration(d)
	}
}
