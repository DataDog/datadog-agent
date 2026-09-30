// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"encoding/hex"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// defaultErrorLogBurst is the number of scan error warnings logged before rate limiting starts
	defaultErrorLogBurst = 10
	// defaultErrorLogInterval is the minimum interval between two scan error warnings once the
	// burst is spent
	defaultErrorLogInterval = time.Minute
)

// StructuredReporterOptions configures a StructuredReporter
type StructuredReporterOptions struct {
	// ErrorLogBurst is the number of scan error warnings logged before rate limiting starts.
	// Zero means defaultErrorLogBurst.
	ErrorLogBurst int
	// ErrorLogInterval is the minimum interval between two scan error warnings once the burst is
	// spent. Zero means defaultErrorLogInterval.
	ErrorLogInterval time.Duration
}

// StructuredReporter is the production Reporter. It writes:
//   - one Info line per scan with matches, with every matched rule, namespace and tag
//   - one rate-limited Warn line per failed scan; suppressed warnings are counted and the count
//     is reported on the next warning that gets through
//   - one Debug line per scan without matches
//
// Every line is a "yara: <outcome>" prefix followed by key=value fields, in the style of the
// other system-probe logs. The path is quoted because it is attacker-controlled and could
// otherwise inject fake fields or lines.
//
// Metrics are not emitted here: every stage counts in Stats, and Metrics emits them.
type StructuredReporter struct {
	rulesVersion string
	errLimit     *log.Limit
	suppressed   atomic.Int64
}

// NewStructuredReporter returns a StructuredReporter tagging every report with rulesVersion,
// usually Scanner.RulesVersion()
func NewStructuredReporter(rulesVersion string, opts StructuredReporterOptions) *StructuredReporter {
	if opts.ErrorLogBurst <= 0 {
		opts.ErrorLogBurst = defaultErrorLogBurst
	}
	if opts.ErrorLogInterval <= 0 {
		opts.ErrorLogInterval = defaultErrorLogInterval
	}
	return &StructuredReporter{
		rulesVersion: rulesVersion,
		errLimit:     log.NewLogLimit(opts.ErrorLogBurst, opts.ErrorLogInterval),
	}
}

// Report implements Reporter
func (r *StructuredReporter) Report(f ExecFile, sum [32]byte, matches []Match, err error) {
	switch {
	case err != nil:
		if !r.errLimit.ShouldLog() {
			r.suppressed.Add(1)
			return
		}
		log.Warn(formatError(f, sum, r.rulesVersion, err, r.suppressed.Swap(0)))
	case len(matches) == 0:
		if log.ShouldLog(log.DebugLvl) {
			log.Debug(formatNoMatch(f, sum, r.rulesVersion))
		}
	default:
		log.Info(formatMatch(f, sum, r.rulesVersion, matches))
	}
}

// formatMatch returns the log line of a scan with matches. Rules are listed in match order;
// namespaces and tags are deduplicated and sorted.
func formatMatch(f ExecFile, sum [32]byte, rulesVersion string, matches []Match) string {
	rules := make([]string, 0, len(matches))
	var namespaces, tags []string
	for _, m := range matches {
		rules = append(rules, m.Rule)
		namespaces = append(namespaces, m.Namespace)
		tags = append(tags, m.Tags...)
	}

	var b strings.Builder
	b.WriteString("yara: match ")
	writeFileFields(&b, f, sum, rulesVersion)
	b.WriteString(" rules=")
	b.WriteString(strings.Join(rules, ","))
	b.WriteString(" namespaces=")
	b.WriteString(strings.Join(sortedUnique(namespaces), ","))
	b.WriteString(" tags=")
	b.WriteString(strings.Join(sortedUnique(tags), ","))
	return b.String()
}

// formatNoMatch returns the log line of a scan without matches
func formatNoMatch(f ExecFile, sum [32]byte, rulesVersion string) string {
	var b strings.Builder
	b.WriteString("yara: no match ")
	writeFileFields(&b, f, sum, rulesVersion)
	return b.String()
}

// formatError returns the log line of a failed scan. suppressed is the number of error lines
// dropped by the rate limiter since the previous one.
func formatError(f ExecFile, sum [32]byte, rulesVersion string, err error, suppressed int64) string {
	var b strings.Builder
	b.WriteString("yara: scan failed ")
	writeFileFields(&b, f, sum, rulesVersion)
	b.WriteString(" error=")
	b.WriteString(strconv.Quote(err.Error()))
	if suppressed > 0 {
		b.WriteString(" suppressed_errors=")
		b.WriteString(strconv.FormatInt(suppressed, 10))
	}
	return b.String()
}

func writeFileFields(b *strings.Builder, f ExecFile, sum [32]byte, rulesVersion string) {
	b.WriteString("path=")
	b.WriteString(strconv.Quote(f.Path))
	b.WriteString(" pid=")
	b.WriteString(strconv.FormatUint(uint64(f.PID), 10))
	b.WriteString(" container_id=")
	b.WriteString(string(f.ContainerID))
	b.WriteString(" script=")
	if f.IsScript {
		b.WriteString("true")
	} else {
		b.WriteString("false")
	}
	b.WriteString(" sha256=")
	b.WriteString(hex.EncodeToString(sum[:]))
	b.WriteString(" rules_version=")
	b.WriteString(rulesVersion)
}

func sortedUnique(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}
