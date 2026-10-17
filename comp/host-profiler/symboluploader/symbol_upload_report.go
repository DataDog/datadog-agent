// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package symboluploader

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
)

const (
	reportInterval   = 5 * time.Minute
	maxReportSymbols = 100
)

var (
	ticker     clockwork.Ticker
	tickerDone chan struct{}
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

// symbolReporter represent the state of a report for a given log level.
type symbolReporter struct {
	mu      sync.Mutex
	clock   clockwork.Clock
	level   slog.Level
	start   time.Time
	reasons map[string]reasonStats
	sampled int
}

func newSymbolReporter(level slog.Level) *symbolReporter {
	clock := clockwork.NewRealClock()
	return &symbolReporter{
		clock:   clock,
		start:   clock.Now(),
		level:   level,
		reasons: make(map[string]reasonStats),
	}
}

// record batches new occurrences of a reason into the report.
// n reprents the number of occurrences of the reason.
// paths are optional binary-path samples, capped at maxReportSymbols entries per report.
func (r *symbolReporter) record(reason string, n int, paths ...string) {
	if n <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	stats := r.reasons[reason]
	sampled := 0
	for _, p := range paths {
		if r.sampled >= maxReportSymbols {
			r.reasons[reason] = stats
			r.emitLocked(r.clock.Now())
			stats = r.reasons[reason]
		}
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		r.sampled++
		sampled++
		stats.count++
		stats.paths = append(stats.paths, p)
	}
	if leftover := n - sampled; leftover > 0 {
		stats.count += leftover
	}
	r.reasons[reason] = stats
}

// flush empties the report and starts a new period.
func (r *symbolReporter) flush(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.emitLocked(now)
	r.start = now
	r.reasons = make(map[string]reasonStats)
	r.sampled = 0
}

// emitLocked emits the report for the current period.
func (r *symbolReporter) emitLocked(now time.Time) {
	if len(r.reasons) == 0 {
		return
	}
	logAttrs := make([]slog.Attr, 0, len(r.reasons)*2+2)
	logAttrs = append(logAttrs,
		slog.String("window_start", r.start.UTC().Format(time.RFC3339Nano)),
		slog.String("window_end", now.UTC().Format(time.RFC3339Nano)))
	for reason, stats := range r.reasons {
		stage, suffix, _ := strings.Cut(reason, ".")
		base := "status_by_reason." + stage + "." + suffix
		logAttrs = append(logAttrs, slog.Int(base+".count", stats.count))
		if len(stats.paths) > 0 {
			logAttrs = append(logAttrs, slog.Any(base+".paths", stats.paths))
		}
	}
	slog.LogAttrs(context.Background(), r.level, "Symbol Upload Report", logAttrs...)
	r.reasons = make(map[string]reasonStats)
	r.sampled = 0
}

type symbolUpload struct{}

var SymbolUpload = symbolUpload{}

// Info batches new occurrences of a reason into the info level report.
func (symbolUpload) Info(reason string, n int, paths ...string) {
	symbolUploadInfo.record(reason, n, paths...)
}

// Error batches new occurrences of a reason into the error level report.
func (symbolUpload) Error(reason string, n int, paths ...string) {
	symbolUploadError.record(reason, n, paths...)
}

func (symbolUpload) Stop() {
	if tickerDone != nil {
		close(tickerDone)
		ticker = nil
		tickerDone = nil
	}
	now := symbolUploadInfo.clock.Now()
	symbolUploadInfo.flush(now)
	symbolUploadError.flush(now)
}

var (
	symbolUploadInfo  = newSymbolReporter(slog.LevelInfo)
	symbolUploadError = newSymbolReporter(slog.LevelError)
)

// startReportFlushTicker starts a ticker that flushes the pending report
// every reportInterval.
func startReportFlushTicker() {
	ticker = symbolUploadInfo.clock.NewTicker(reportInterval)
	tickerDone = make(chan struct{})
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-tickerDone:
				return
			case <-ticker.Chan():
				now := symbolUploadInfo.clock.Now()
				symbolUploadInfo.flush(now)
				symbolUploadError.flush(now)
			}
		}
	}()
}
