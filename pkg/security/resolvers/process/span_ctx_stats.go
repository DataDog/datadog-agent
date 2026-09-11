// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"errors"
	"fmt"
	"syscall"

	"go.uber.org/atomic"

	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/otelprocessctx"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/golabelsctx"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/otelattrs"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
)

// Sentinels a resolution step wraps its error with to steer classifySpanCtxError,
// for outcomes that carry no syscall errno of their own to fold.
var (
	// errSpanCtxNotApplicable marks a process that is instrumented, but not
	// through the reader that returned this error.
	errSpanCtxNotApplicable = errors.New("not applicable")
	// errSpanCtxUnsupported marks a shape this code recognizes but cannot handle
	errSpanCtxUnsupported = errors.New("unsupported")
	// errSpanCtxMalformed marks a process that violated the contract it published
	// under
	errSpanCtxMalformed = errors.New("malformed")
	// errSpanCtxGone marks a target that stopped existing mid-resolution
	errSpanCtxGone = errors.New("gone")
	// errSpanCtxMapError marks a failure to push resolved offsets into a BPF map.
	errSpanCtxMapError = errors.New("map error")
)

// spanCtxStatus classifies the outcome of a span context resolution step.
type spanCtxStatus int

const (
	spanCtxOK spanCtxStatus = iota
	// spanCtxNotApplicable: instrumented, but not through this reader
	spanCtxNotApplicable
	// spanCtxUnsupported: a shape we know and can't handle
	spanCtxUnsupported
	// spanCtxMalformed: the process published something that violates the
	// contract.
	spanCtxMalformed
	// spanCtxUnpublished: seqlock timestamp 0 -- publisher mid-update or not yet
	// published.
	spanCtxUnpublished
	// spanCtxTorn: the payload changed under us on every attempt.
	spanCtxTorn
	// spanCtxUnreadable: permission/namespace -- /proc/<pid>/mem is not readable.
	spanCtxUnreadable
	// spanCtxGone: the process exited while we were resolving.
	spanCtxGone
	// spanCtxQueueFull: the resolution request was dropped.
	spanCtxQueueFull
	// spanCtxMapError: pushing the offsets into the BPF map failed.
	spanCtxMapError
	// spanCtxNoProcessEntry: the kernel says it published, we have no cache entry
	// for it.
	spanCtxNoProcessEntry
	// spanCtxStaleID: the ring slot's id no longer matches the id the event
	// carried -- the slot was reused before this event was resolved.
	spanCtxStaleID
	spanCtxUnknown
	// spanCtxLast must stay last
	spanCtxLast
)

func (s spanCtxStatus) String() string {
	switch s {
	case spanCtxOK:
		return "ok"
	case spanCtxNotApplicable:
		return "not_applicable"
	case spanCtxUnsupported:
		return "unsupported"
	case spanCtxMalformed:
		return "malformed"
	case spanCtxUnpublished:
		return "unpublished"
	case spanCtxTorn:
		return "torn"
	case spanCtxUnreadable:
		return "unreadable"
	case spanCtxGone:
		return "gone"
	case spanCtxQueueFull:
		return "queue_full"
	case spanCtxMapError:
		return "map_error"
	case spanCtxNoProcessEntry:
		return "no_process_entry"
	case spanCtxStaleID:
		return "stale_id"
	default:
		return "unknown"
	}
}

// Tag returns the statsd "status:" tag for s.
func (s spanCtxStatus) Tag() string {
	return "status:" + s.String()
}

// expected reports whether s is an outcome we can't act on -- it drives the log
// level in reportSpanCtxError.
func (s spanCtxStatus) expected() bool {
	switch s {
	case spanCtxOK, spanCtxNotApplicable, spanCtxUnpublished, spanCtxGone, spanCtxNoProcessEntry, spanCtxStaleID:
		return true
	default:
		return false
	}
}

// classifySpanCtxError maps an error to a status
func classifySpanCtxError(err error) spanCtxStatus {
	switch {
	case err == nil:
		return spanCtxOK
	case errors.Is(err, errSpanCtxNotApplicable):
		return spanCtxNotApplicable
	case errors.Is(err, errSpanCtxUnsupported):
		return spanCtxUnsupported
	case errors.Is(err, errSpanCtxMalformed):
		return spanCtxMalformed
	case errors.Is(err, errSpanCtxGone):
		return spanCtxGone
	case errors.Is(err, errSpanCtxMapError):
		return spanCtxMapError
	case errors.Is(err, otelprocessctx.ErrUnpublished):
		return spanCtxUnpublished
	case errors.Is(err, otelprocessctx.ErrTorn):
		return spanCtxTorn
	case errors.Is(err, otelprocessctx.ErrUnsupportedVersion):
		return spanCtxUnsupported
	case errors.Is(err, otelprocessctx.ErrMalformed):
		return spanCtxMalformed
	case errors.Is(err, golabelsctx.ErrMapLookup), errors.Is(err, otelattrs.ErrMapLookup):
		return spanCtxMapError
	case errors.Is(err, golabelsctx.ErrStaleID), errors.Is(err, otelattrs.ErrStaleID):
		return spanCtxStaleID
	case errors.Is(err, otelattrs.ErrMalformed):
		return spanCtxMalformed
	case errors.Is(err, syscall.ESRCH), errors.Is(err, syscall.ENOENT), errors.Is(err, syscall.EIO):
		return spanCtxGone
	case errors.Is(err, syscall.EACCES), errors.Is(err, syscall.EPERM):
		return spanCtxUnreadable
	default:
		return spanCtxUnknown
	}
}

// spanCtxStep identifies which span context resolution step an outcome
// belongs to.
type spanCtxStep int

const (
	// spanCtxStepProcessCtx is reading a process's OTel process context (OTEP
	// 4719).
	spanCtxStepProcessCtx spanCtxStep = iota
	// spanCtxStepOTelTLS is installing the OTel TLS reader.
	spanCtxStepOTelTLS
	// spanCtxStepGoLabels is installing the Go pprof-labels reader.
	spanCtxStepGoLabels
	// spanCtxStepGoLabelsLookup is a per-event golabelsctx.Resolver.Resolve call.
	spanCtxStepGoLabelsLookup
	// spanCtxStepOTelAttrsLookup is a per-event otelattrs.Resolver.Resolve call.
	spanCtxStepOTelAttrsLookup
	// spanCtxStepLast must stay last
	spanCtxStepLast
)

func (s spanCtxStep) String() string {
	switch s {
	case spanCtxStepProcessCtx:
		return "OTel process context read"
	case spanCtxStepOTelTLS:
		return "OTel TLS resolution"
	case spanCtxStepGoLabels:
		return "Go labels resolution"
	case spanCtxStepGoLabelsLookup:
		return "Go labels lookup"
	case spanCtxStepOTelAttrsLookup:
		return "OTel attrs lookup"
	default:
		return "span context resolution"
	}
}

// reader is the "reader:" tag value for s if applicable.
func (s spanCtxStep) reader() string {
	switch s {
	case spanCtxStepOTelTLS, spanCtxStepOTelAttrsLookup:
		return "otel_tls"
	case spanCtxStepGoLabels, spanCtxStepGoLabelsLookup:
		return "go_labels"
	default:
		return ""
	}
}

func (s spanCtxStep) metricPair() (success, failed string) {
	switch s {
	case spanCtxStepOTelTLS, spanCtxStepGoLabels:
		return metrics.MetricSpanContextResolutionSuccess, metrics.MetricSpanContextResolutionFailed
	case spanCtxStepGoLabelsLookup, spanCtxStepOTelAttrsLookup:
		return metrics.MetricSpanContextEventSuccess, metrics.MetricSpanContextEventFailed
	default:
		return metrics.MetricSpanContextProcessCtxSuccess, metrics.MetricSpanContextProcessCtxFailed
	}
}

// spanCtxKey is the counter map key: one counter per (step, status) pair.
type spanCtxKey struct {
	step   spanCtxStep
	status spanCtxStatus
}

// metric is the statsd metric name s is reported under.
func (s spanCtxKey) metric() string {
	success, failed := s.step.metricPair()

	if s.status == spanCtxOK {
		return success
	}
	return failed
}

// newSpanCtxStats builds the span context counters upfront.
func newSpanCtxStats() map[spanCtxKey]*atomic.Int64 {
	stats := make(map[spanCtxKey]*atomic.Int64)
	for step := spanCtxStepProcessCtx; step < spanCtxStepLast; step++ {
		for status := spanCtxOK; status < spanCtxLast; status++ {
			stats[spanCtxKey{step: step, status: status}] = atomic.NewInt64(0)
		}
	}
	return stats
}

// countSpanCtx counts one outcome of step. The counters are flushed by
// sendSpanCtxStats.
func (p *EBPFResolver) countSpanCtx(step spanCtxStep, status spanCtxStatus) {
	if counter, ok := p.spanCtxStats[spanCtxKey{step: step, status: status}]; ok {
		counter.Inc()
	}
}

// sendSpanCtxStats flushes the span context counters to the statsd client.
func (p *EBPFResolver) sendSpanCtxStats() error {
	for key, counter := range p.spanCtxStats {
		count := counter.Swap(0)
		if count == 0 {
			continue
		}

		tags := []string{key.status.Tag()}
		if reader := key.step.reader(); reader != "" {
			tags = append(tags, "reader:"+reader)
		}

		if err := p.statsdClient.Count(key.metric(), count, tags, 1.0); err != nil {
			return fmt.Errorf("failed to send span context metric for %s: %w", key.step, err)
		}
	}
	return nil
}

// reportSpanCtx classifies and counts the outcome of step for pid, logging a
// success line on nil and delegating to reportSpanCtxError otherwise.
func (p *EBPFResolver) reportSpanCtx(step spanCtxStep, pid uint32, err error) {
	if err == nil {
		p.countSpanCtx(step, spanCtxOK)
		seclog.Debugf("%s succeeded for pid %d", step, pid)
		return
	}
	p.reportSpanCtxError(step, pid, err)
}

// reportSpanCtxError classifies, counts and logs a span context resolution
// failure.
func (p *EBPFResolver) reportSpanCtxError(step spanCtxStep, pid uint32, err error) {
	status := classifySpanCtxError(err)
	p.countSpanCtx(step, status)

	if status.expected() {
		seclog.Debugf("%s for pid %d: %s [%s]", step, pid, err, status)
	} else {
		seclog.Warnf("%s for pid %d: %s [%s]", step, pid, err, status)
	}
}

// countLookup classifies and counts the outcome of a per-event lookup step.
func (p *EBPFResolver) countLookup(step spanCtxStep, err error) {
	status := classifySpanCtxError(err)
	p.countSpanCtx(step, status)

	if err == nil {
		return
	}
	if status.expected() {
		seclog.Debugf("%s: %s [%s]", step, err, status)
	} else {
		seclog.Warnf("%s: %s [%s]", step, err, status)
	}
}

// CountGoLabelsLookup counts the outcome of a per-event Go pprof-labels
// lookup (golabelsctx.Resolver.Resolve).
func (p *EBPFResolver) CountGoLabelsLookup(err error) {
	p.countLookup(spanCtxStepGoLabelsLookup, err)
}

// CountOTelAttrsLookup counts the outcome of a per-event OTel attributes
// lookup (otelattrs.Resolver.Resolve).
func (p *EBPFResolver) CountOTelAttrsLookup(err error) {
	p.countLookup(spanCtxStepOTelAttrsLookup, err)
}
