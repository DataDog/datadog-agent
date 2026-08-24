// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/security/metrics"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// Span context tracking has two independent readers -- Go pprof labels
// (go_labels.go) and OTel thread local context records (otel_tls.go) -- and two
// stages. Resolving works out how to read a traced process, once per process.
// Reading turns what the kernel captured into a span context, once per event.
//
// A failure in either stage is invisible everywhere else: the process is still
// tracked and its events are still emitted, they just carry no span context. So
// both stages report, under their own metric, tagged with the tracer language
// and with what went wrong.

// spanTrackingReason is the `reason` tag of MetricSpanTrackingResolveError.
type spanTrackingReason int

const (
	// spanTrackingReasonNone marks a failure that must not be reported at all:
	// a tracer that does not implement the feature is the expected outcome for
	// most processes, and counting it would bury the failures worth acting on.
	spanTrackingReasonNone spanTrackingReason = iota
	// spanTrackingReasonUnknown is the fallback for an error that reached the
	// resolver without being classified.
	spanTrackingReasonUnknown
	// spanTrackingReasonMapUnavailable is a missing eBPF map, so the offsets
	// have nowhere to be written.
	spanTrackingReasonMapUnavailable
	// spanTrackingReasonMapPut is a failed write of the resolved offsets, most
	// likely a full map.
	spanTrackingReasonMapPut
	// spanTrackingReasonProcExe is an unreadable /proc/<pid>/exe, usually a
	// process that exited mid-resolution.
	spanTrackingReasonProcExe
	// spanTrackingReasonProcMaps is an unreadable /proc/<pid>/maps.
	spanTrackingReasonProcMaps
	// spanTrackingReasonBuildInfo is a Go binary whose .go.buildinfo could not
	// be read, so its runtime version is unknown.
	spanTrackingReasonBuildInfo
	// spanTrackingReasonUnsupportedVersion is a Go version outside the range we
	// hold struct offsets for. It is the reason to watch: every Go release adds
	// to it until the offset table catches up.
	spanTrackingReasonUnsupportedVersion
	// spanTrackingReasonTLSOffset is a Go binary whose g TLS offset could not be
	// decoded.
	spanTrackingReasonTLSOffset
	// spanTrackingReasonBadSymbol is an otel_thread_ctx_v1 that is present but
	// not what the spec describes, which points at the writer rather than at us.
	spanTrackingReasonBadSymbol
	// spanTrackingReasonTLSAccess is an otel_thread_ctx_v1 whose access model
	// could not be classified from its defining ELF object.
	spanTrackingReasonTLSAccess
	// spanTrackingReasonTLSAttach is a failure to read the loader-resolved TLS
	// slot out of the live process, which includes walking the DTV.
	spanTrackingReasonTLSAttach
	spanTrackingReasonCount
)

// spanTrackingReasonNames holds the `reason` tag value of each reason. The None
// entry is never read: countSpanTrackingResolveError drops those before it.
var spanTrackingReasonNames = [spanTrackingReasonCount]string{
	spanTrackingReasonNone:               "",
	spanTrackingReasonUnknown:            "unknown",
	spanTrackingReasonMapUnavailable:     "map_unavailable",
	spanTrackingReasonMapPut:             "map_put",
	spanTrackingReasonProcExe:            "proc_exe",
	spanTrackingReasonProcMaps:           "proc_maps",
	spanTrackingReasonBuildInfo:          "build_info",
	spanTrackingReasonUnsupportedVersion: "unsupported_version",
	spanTrackingReasonTLSOffset:          "tls_offset",
	spanTrackingReasonBadSymbol:          "bad_symbol",
	spanTrackingReasonTLSAccess:          "tls_access",
	spanTrackingReasonTLSAttach:          "tls_attach",
}

func (r spanTrackingReason) String() string { return spanTrackingReasonNames[r] }

// spanTrackingLang is the `lang` tag of the span tracking metrics.
type spanTrackingLang int

const (
	spanTrackingLangOther spanTrackingLang = iota
	spanTrackingLangGo
	spanTrackingLangJava
	spanTrackingLangDotnet
	spanTrackingLangNodeJS
	spanTrackingLangPython
	spanTrackingLangRuby
	spanTrackingLangPHP
	spanTrackingLangCPP
	spanTrackingLangRust
	spanTrackingLangCount
)

// spanTrackingLangNames holds the `lang` tag value of each language.
var spanTrackingLangNames = [spanTrackingLangCount]string{
	spanTrackingLangOther:  "other",
	spanTrackingLangGo:     "go",
	spanTrackingLangJava:   "java",
	spanTrackingLangDotnet: "dotnet",
	spanTrackingLangNodeJS: "nodejs",
	spanTrackingLangPython: "python",
	spanTrackingLangRuby:   "ruby",
	spanTrackingLangPHP:    "php",
	spanTrackingLangCPP:    "cpp",
	spanTrackingLangRust:   "rust",
}

func (l spanTrackingLang) String() string { return spanTrackingLangNames[l] }

// spanTrackingLangs maps a TracerMetadata language onto the closed set above.
var spanTrackingLangs = map[string]spanTrackingLang{
	"go":     spanTrackingLangGo,
	"java":   spanTrackingLangJava,
	"jvm":    spanTrackingLangJava,
	"dotnet": spanTrackingLangDotnet,
	"node":   spanTrackingLangNodeJS,
	"nodejs": spanTrackingLangNodeJS,
	"python": spanTrackingLangPython,
	"ruby":   spanTrackingLangRuby,
	"php":    spanTrackingLangPHP,
	"cpp":    spanTrackingLangCPP,
	"rust":   spanTrackingLangRust,
}

// spanTrackingLangOf classifies a tracer language. The language comes from a
// memfd the traced process writes itself, so an unrecognized value collapses
// into `other` rather than becoming a tag value of its own.
func spanTrackingLangOf(tracerLanguage string) spanTrackingLang {
	if lang, ok := spanTrackingLangs[tracerLanguage]; ok {
		return lang
	}
	return spanTrackingLangOther
}

// spanTrackingError carries the reason a resolution failed alongside the error,
// so that the classification is made where the failure happens rather than
// recovered from the message at the call site.
type spanTrackingError struct {
	reason spanTrackingReason
	err    error
}

func (e *spanTrackingError) Error() string { return e.err.Error() }

func (e *spanTrackingError) Unwrap() error { return e.err }

// spanTrackingErrorf builds an error tagged with the reason it is reported under.
func spanTrackingErrorf(reason spanTrackingReason, format string, args ...any) error {
	return &spanTrackingError{reason: reason, err: fmt.Errorf(format, args...)}
}

// spanTrackingWrap tags an existing error with the reason it is reported under.
func spanTrackingWrap(reason spanTrackingReason, err error) error {
	return &spanTrackingError{reason: reason, err: err}
}

// spanTrackingReasonOf recovers the reason err was tagged with.
func spanTrackingReasonOf(err error) spanTrackingReason {
	var ste *spanTrackingError
	if errors.As(err, &ste) {
		return ste.reason
	}
	return spanTrackingReasonUnknown
}

var spanTrackingTelemetry = struct {
	resolveError   telemetry.Counter
	resolveSuccess telemetry.Counter
	readError      telemetry.Counter
	readSuccess    telemetry.Counter
}{
	resolveError:   metrics.NewITCounter(metrics.MetricSpanTrackingResolveError, []string{"lang", "reason"}, "Number of processes a span context reader could not be set up for"),
	resolveSuccess: metrics.NewITCounter(metrics.MetricSpanTrackingResolveSuccess, []string{"lang"}, "Number of processes a span context reader was set up for"),
	readError:      metrics.NewITCounter(metrics.MetricSpanTrackingReadError, []string{"lang", "reason"}, "Number of events whose span context a reader failed to capture"),
	readSuccess:    metrics.NewITCounter(metrics.MetricSpanTrackingReadSuccess, []string{"lang"}, "Number of events that came out of the kernel with a span context"),
}

// countSpanTrackingResolveError records a process a reader could not be set up
// for, whose tracer language is lang.
func countSpanTrackingResolveError(lang spanTrackingLang, err error) {
	reason := spanTrackingReasonOf(err)
	if reason == spanTrackingReasonNone {
		return
	}
	spanTrackingTelemetry.resolveError.Inc(lang.String(), reason.String())
}

// countSpanTrackingResolveSuccess records a process set up to be read from.
func countSpanTrackingResolveSuccess(lang spanTrackingLang) {
	spanTrackingTelemetry.resolveSuccess.Inc(lang.String())
}

// CountSpanContextRead reports what the kernel readers left on one event.
func CountSpanContextRead(tracerLanguage string, kernelError model.SpanContextError, hasSpan bool) {
	lang := spanTrackingLangOf(tracerLanguage).String()

	switch {
	case kernelError != model.SpanContextNoError:
		spanTrackingTelemetry.readError.Inc(lang, kernelError.String())
	case hasSpan:
		spanTrackingTelemetry.readSuccess.Inc(lang)
	}
}
