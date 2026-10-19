// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package symboluploader

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

const (
	reportMessage  = "Symbol Upload Report"
	reportInterval = 5 * time.Minute
	maxReportPaths = 100
)

// Report reason taxonomy. Reasons are prefixed by their pipeline stage.
const (
	reasonAlreadyUploaded  = "retrieval.already_uploaded"
	reasonNoDebugSymbols   = "retrieval.no_debug_symbols"
	reasonQueueFull        = "retrieval.queue_full"
	reasonGoPCLnTabError   = "retrieval.gopclntab_error"
	reasonElfReadError     = "retrieval.elf_read_error"
	reasonExistingSymbols  = "upload.existing_symbols"
	reasonUploaded         = "upload.uploaded"
	reasonExtractError     = "upload.extract_error"
	reasonIntakeError      = "upload.intake_error"
	reasonQueried          = "query.queried"
	reasonSourceParseError = "query.source_parse_error"
	reasonInvalidBuildID   = "query.invalid_build_id"
	reasonRateLimited      = "query.rate_limited"
	reasonRequestError     = "query.request_error"
)

type reasonStats struct {
	count int
	paths []string
}

type pendingReport struct {
	start   time.Time
	end     time.Time
	reasons map[string]reasonStats
	sampled int
}

func (p *pendingReport) emit(level slog.Level) {
	logAttrs := make([]slog.Attr, 0, len(p.reasons)*2+2)
	logAttrs = append(logAttrs,
		slog.String("window_start", p.start.UTC().Format(time.RFC3339Nano)),
		slog.String("window_end", p.end.UTC().Format(time.RFC3339Nano)))
	for reason, stats := range p.reasons {
		base := "status_by_reason." + reason
		logAttrs = append(logAttrs, slog.Int(base+".count", stats.count))
		if len(stats.paths) > 0 {
			logAttrs = append(logAttrs, slog.Any(base+".paths", stats.paths))
		}
	}
	slog.LogAttrs(context.Background(), level, reportMessage, logAttrs...)
}

// symbolReporter accumulates report occurrences at level into its current
// pendingReport.
type symbolReporter struct {
	mu      sync.Mutex
	level   slog.Level
	current pendingReport
}

func newSymbolReporter(level slog.Level) *symbolReporter {
	return &symbolReporter{
		level: level,
		current: pendingReport{
			start:   time.Now(),
			reasons: make(map[string]reasonStats),
		},
	}
}

// takePendingReportLocked returns the current pending report and installs a fresh one.
func (r *symbolReporter) takePendingReportLocked() *pendingReport {
	now := time.Now()
	pending := r.current
	pending.end = now
	r.current = pendingReport{
		start:   now,
		reasons: make(map[string]reasonStats),
	}
	if len(pending.reasons) == 0 {
		return nil
	}
	return &pending
}

// record batches new occurrences of a reason into the report.
// n represents the number of occurrences of the reason.
// paths are optional binary-path samples.
// When sampling maxReportPaths paths, we emit the full report and start a new one.
func (r *symbolReporter) record(reason string, n int, paths ...string) {
	if n <= 0 {
		return
	}
	var toEmit []*pendingReport
	r.mu.Lock()
	added := 0
	for _, p := range paths {
		if p == "" {
			continue
		}
		if r.current.sampled >= maxReportPaths {
			toEmit = append(toEmit, r.takePendingReportLocked())
		}
		stats := r.current.reasons[reason]
		stats.count++
		stats.paths = append(stats.paths, p)
		r.current.reasons[reason] = stats

		r.current.sampled++
		added++
	}
	if leftover := n - added; leftover > 0 {
		stats := r.current.reasons[reason]
		stats.count += leftover
		r.current.reasons[reason] = stats
	}
	r.mu.Unlock()

	for _, report := range toEmit {
		report.emit(r.level)
	}
}

// flush empties the pending report and starts a new period.
func (r *symbolReporter) flush() {
	r.mu.Lock()
	report := r.takePendingReportLocked()
	r.mu.Unlock()

	if report != nil {
		report.emit(r.level)
	}
}

var (
	infoReporter  = newSymbolReporter(slog.LevelInfo)
	errorReporter = newSymbolReporter(slog.LevelError)
)

// reportInfo batches n occurrences of reason into the info level report.
func reportInfo(reason string, n int, paths ...string) {
	infoReporter.record(reason, n, paths...)
}

// reportError batches n occurrences of reason into the error level report.
func reportError(reason string, n int, paths ...string) {
	errorReporter.record(reason, n, paths...)
}

// flushReports emits the pending info and error reports and starts a new period.
func flushReports() {
	infoReporter.flush()
	errorReporter.flush()
}

// startReportFlushTicker flushes the pending reports of the package-level
// reporters every reportInterval, until the returned stop function
// is called.
func startReportFlushTicker() (stop func()) {
	ticker := time.NewTicker(reportInterval)
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				flushReports()
			}
		}
	})
	return sync.OnceFunc(func() {
		close(done)
		wg.Wait()
	})
}
