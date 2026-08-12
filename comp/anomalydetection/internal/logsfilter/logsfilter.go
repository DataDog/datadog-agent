// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package logsfilter provides shared log-filtering primitives (severity
// bucketing and fixed-window rate limiting) used by both the observer and
// logssource components of the anomaly-detection subsystem.
package logsfilter

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// RateLimitDiscrepancies reports rate-limit settings that would make a less
// severe log tier more permissive than a more severe one. A configured value
// of -1 is displayed as +Inf for ordering comparisons. Values below -1 are
// unsupported, even though the runtime limiter currently treats any negative
// value as unlimited.
func RateLimitDiscrepancies(low, medium, high float64) []string {
	values := []struct {
		name  string
		value float64
	}{
		{"low", low},
		{"medium", medium},
		{"high", high},
	}

	var discrepancies []string
	for _, value := range values {
		if value.value < -1 {
			discrepancies = append(discrepancies, fmt.Sprintf("max_rate_%s_priority must be -1 or >= 0, got %g", value.name, value.value))
		}
	}

	normalizedLow := normalizeRateLimit(low)
	normalizedMedium := normalizeRateLimit(medium)
	normalizedHigh := normalizeRateLimit(high)
	if normalizedLow > normalizedMedium || normalizedMedium > normalizedHigh {
		discrepancies = append(discrepancies, fmt.Sprintf("max_rate priority limits should be non-decreasing (low=%g, medium=%g, high=%g)", normalizedLow, normalizedMedium, normalizedHigh))
	}
	return discrepancies
}

// WarnRateLimitDiscrepancies emits configuration warnings for a source's
// priority-tier limits. Each warning includes every fully-qualified key used
// by the ordering check.
func WarnRateLimitDiscrepancies(prefix string, low, medium, high float64) {
	for _, discrepancy := range RateLimitDiscrepancies(low, medium, high) {
		pkglog.Warnf("config max rates within %s: %s", prefix, discrepancy)
	}
}

func normalizeRateLimit(rate float64) float64 {
	if rate < 0 {
		return math.Inf(1)
	}
	return rate
}

// RateWindowDuration is the fixed window size used by RateWindow.
const RateWindowDuration = 10 * time.Second

// PriorityBucket represents a log's severity level as a comparable integer.
// Higher value = more severe. The numeric values mirror pkg/util/log/types.LogLevel
// (itself based on slog.Level) so the ordering is consistent with the agent's
// own log-level system — copied here to avoid a dependency on that package.
type PriorityBucket int

const (
	TracePriority    PriorityBucket = -8 // slog.LevelDebug - 4
	DebugPriority    PriorityBucket = -4 // slog.LevelDebug
	InfoPriority     PriorityBucket = 0  // slog.LevelInfo
	WarnPriority     PriorityBucket = 4  // slog.LevelWarn
	ErrorPriority    PriorityBucket = 8  // slog.LevelError
	CriticalPriority PriorityBucket = 12 // slog.LevelError + 4
	OffPriority      PriorityBucket = 16 // slog.LevelError + 8  — blocks everything
)

// BucketForStatus maps a log status string to a PriorityBucket.
// Unrecognised status strings are treated as InfoPriority so they are
// forwarded under the medium (info) rate budget.
func BucketForStatus(status string) PriorityBucket {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "emergency", "alert", "critical", "fatal":
		return CriticalPriority
	case "error":
		return ErrorPriority
	case "warn", "warning", "notice":
		return WarnPriority
	case "debug":
		return DebugPriority
	case "trace":
		return TracePriority
	default: // info, and any unrecognised string
		return InfoPriority
	}
}

// MinBucketForSeverity maps a min_severity config value to the lowest
// PriorityBucket a log must reach to be forwarded.
//
//   - "" (empty) → passes everything (TracePriority - 1)
//   - "trace" → TracePriority and above
//   - "debug" → DebugPriority and above
//   - "info" → InfoPriority and above
//   - "warn" / "warning" → WarnPriority and above
//   - "error" → ErrorPriority and above
//   - "critical" / "fatal" / equivalents → CriticalPriority and above
//   - "off" → OffPriority (blocks everything)
//   - unrecognised non-empty string → WarnPriority (safe default)
func MinBucketForSeverity(minSeverity string) PriorityBucket {
	bucket, _ := parseMinSeverity(minSeverity)
	return bucket
}

// IsValidMinSeverity reports whether minSeverity is an accepted configuration
// value. Empty is valid and means that no minimum severity is applied.
func IsValidMinSeverity(minSeverity string) bool {
	_, valid := parseMinSeverity(minSeverity)
	return valid
}

// WarnInvalidMinSeverity emits a startup warning for an unsupported
// min_severity value. The caller can continue using MinBucketForSeverity,
// which retains its safe WarnPriority fallback.
func WarnInvalidMinSeverity(key, value string) {
	if !IsValidMinSeverity(value) {
		pkglog.Warnf("config %s has unrecognized value %q; using the safe default warn", key, value)
	}
}

func parseMinSeverity(minSeverity string) (PriorityBucket, bool) {
	switch strings.ToLower(strings.TrimSpace(minSeverity)) {
	case "":
		return TracePriority - 1, true // below TracePriority → all logs pass
	case "trace":
		return TracePriority, true
	case "debug":
		return DebugPriority, true
	case "info":
		return InfoPriority, true
	case "warn", "warning":
		return WarnPriority, true
	case "error":
		return ErrorPriority, true
	case "critical", "fatal", "alert", "emergency":
		return CriticalPriority, true
	case "off":
		return OffPriority, true
	default:
		return WarnPriority, false // safe default for unrecognised values
	}
}

// RateTierForBucket maps a PriorityBucket to one of three rate-limit tiers
// ("low", "medium", "high") used by the max_rate_* config keys.
func RateTierForBucket(b PriorityBucket) string {
	switch {
	case b >= WarnPriority:
		return "high"
	case b >= InfoPriority:
		return "medium"
	default:
		return "low"
	}
}

// RateWindow is a fixed-window rate limiter. It allows at most
// maxRate * RateWindowDuration.Seconds() messages per window.
// A maxRate of -1 means unlimited (no cap); 0 drops all messages.
// The minimum effective rate is 1/RateWindowDuration.Seconds() (0.1/s for a
// 10-second window); rates below that floor allow at most 1 message per window.
type RateWindow struct {
	mu          sync.Mutex
	windowStart time.Time
	count       int64
}

// Allow returns true if the message should be forwarded.
// maxRatePerSec == -1 means unlimited; 0 drops everything.
func (w *RateWindow) Allow(maxRatePerSec float64) bool {
	if maxRatePerSec < 0 {
		return true
	}
	if maxRatePerSec == 0 {
		return false
	}
	// Kept as float64 to avoid silent truncation for sub-0.1/s rates.
	maxPerWindow := maxRatePerSec * RateWindowDuration.Seconds()

	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.windowStart.IsZero() || now.Sub(w.windowStart) >= RateWindowDuration {
		w.windowStart = now
		w.count = 0
	}

	if float64(w.count) < maxPerWindow {
		w.count++
		return true
	}
	return false
}
