// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"sync"
)

// LogBuffer provides memory-efficient log storage with pattern deduplication.
// It keeps one example per unique log pattern plus all error/warn logs.
type LogBuffer struct {
	mu sync.RWMutex

	// Pattern-deduped logs: one example per signature
	patternBuckets map[string]*PatternBucket

	// Error/warn logs always kept (up to maxErrors)
	errorLogs []BufferedLog

	// Configuration
	maxPatterns int   // max unique patterns to track
	maxErrors   int   // max error/warn logs to keep
	windowSec   int64 // evict logs older than this (seconds)
}

// PatternBucket tracks a unique log pattern with one example and count.
type PatternBucket struct {
	Signature string      `json:"signature"`
	Example   BufferedLog `json:"example"`
	Count     int         `json:"count"`
	FirstSeen int64       `json:"firstSeen"`
	LastSeen  int64       `json:"lastSeen"`
	Sources   []string    `json:"sources,omitempty"` // unique sources that emitted this pattern
}

// BufferedLog is a log entry stored in the buffer.
type BufferedLog struct {
	Timestamp int64    `json:"timestamp"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags,omitempty"`
	Source    string   `json:"source,omitempty"`
	Level     string   `json:"level,omitempty"`
}

// LogPatternSummary is the output format for GetLogSummary().
type LogPatternSummary struct {
	Signature string   `json:"signature"`
	Example   string   `json:"example"`
	Count     int      `json:"count"`
	FirstSeen int64    `json:"firstSeen"`
	LastSeen  int64    `json:"lastSeen"`
	Sources   []string `json:"sources,omitempty"`
}

// LogBufferConfig configures the log buffer.
type LogBufferConfig struct {
	MaxPatterns int   // default: 500
	MaxErrors   int   // default: 200
	WindowSec   int64 // default: 300 (5 minutes)
}

// DefaultLogBufferConfig returns sensible defaults.
func DefaultLogBufferConfig() LogBufferConfig {
	return LogBufferConfig{
		MaxPatterns: 500,
		MaxErrors:   200,
		WindowSec:   300,
	}
}

// NewLogBuffer creates a new log buffer with the given config.
func NewLogBuffer(cfg LogBufferConfig) *LogBuffer {
	if cfg.MaxPatterns <= 0 {
		cfg.MaxPatterns = 500
	}
	if cfg.MaxErrors <= 0 {
		cfg.MaxErrors = 200
	}
	if cfg.WindowSec <= 0 {
		cfg.WindowSec = 300
	}

	return &LogBuffer{
		patternBuckets: make(map[string]*PatternBucket),
		errorLogs:      make([]BufferedLog, 0, cfg.MaxErrors),
		maxPatterns:    cfg.MaxPatterns,
		maxErrors:      cfg.MaxErrors,
		windowSec:      cfg.WindowSec,
	}
}

// Add adds a log to the buffer. It deduplicates by pattern signature.
// Error/warn logs are always kept (up to maxErrors).
func (b *LogBuffer) Add(timestamp int64, content string, tags []string, source, level string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	log := BufferedLog{
		Timestamp: timestamp,
		Content:   content,
		Tags:      tags,
		Source:    source,
		Level:     level,
	}

	// Always keep error/warn logs
	if isErrorLevel(level) {
		b.addErrorLog(log)
	}

	// Dedupe by pattern signature
	sig := getLogSignature(content)
	if bucket, exists := b.patternBuckets[sig]; exists {
		// Update existing bucket
		bucket.Count++
		bucket.LastSeen = timestamp
		// Track unique sources
		if source != "" && !containsString(bucket.Sources, source) {
			bucket.Sources = append(bucket.Sources, source)
		}
	} else {
		// New pattern - add if we have room
		if len(b.patternBuckets) < b.maxPatterns {
			b.patternBuckets[sig] = &PatternBucket{
				Signature: sig,
				Example:   log,
				Count:     1,
				FirstSeen: timestamp,
				LastSeen:  timestamp,
				Sources:   []string{source},
			}
		}
		// If at capacity, we could evict oldest pattern, but for now just skip
	}
}

// addErrorLog adds to the error log ring buffer.
func (b *LogBuffer) addErrorLog(log BufferedLog) {
	if len(b.errorLogs) >= b.maxErrors {
		// Shift left, drop oldest
		copy(b.errorLogs, b.errorLogs[1:])
		b.errorLogs = b.errorLogs[:len(b.errorLogs)-1]
	}
	b.errorLogs = append(b.errorLogs, log)
}

// GetLogSummary returns pattern summaries for all tracked patterns.
func (b *LogBuffer) GetLogSummary() []LogPatternSummary {
	b.mu.RLock()
	defer b.mu.RUnlock()

	summaries := make([]LogPatternSummary, 0, len(b.patternBuckets))
	for _, bucket := range b.patternBuckets {
		summaries = append(summaries, LogPatternSummary{
			Signature: bucket.Signature,
			Example:   bucket.Example.Content,
			Count:     bucket.Count,
			FirstSeen: bucket.FirstSeen,
			LastSeen:  bucket.LastSeen,
			Sources:   bucket.Sources,
		})
	}
	return summaries
}

// GetErrorLogs returns all buffered error/warn logs.
func (b *LogBuffer) GetErrorLogs() []BufferedLog {
	b.mu.RLock()
	defer b.mu.RUnlock()

	result := make([]BufferedLog, len(b.errorLogs))
	copy(result, b.errorLogs)
	return result
}

// GetLogsInWindow returns logs (from error buffer) within the time window.
func (b *LogBuffer) GetLogsInWindow(start, end int64) []BufferedLog {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var result []BufferedLog
	for _, log := range b.errorLogs {
		if log.Timestamp >= start && log.Timestamp <= end {
			result = append(result, log)
		}
	}
	return result
}

// GetPatternsInWindow returns pattern summaries active within the time window.
func (b *LogBuffer) GetPatternsInWindow(start, end int64) []LogPatternSummary {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var summaries []LogPatternSummary
	for _, bucket := range b.patternBuckets {
		// Include if pattern was seen within window
		if bucket.LastSeen >= start && bucket.FirstSeen <= end {
			summaries = append(summaries, LogPatternSummary{
				Signature: bucket.Signature,
				Example:   bucket.Example.Content,
				Count:     bucket.Count,
				FirstSeen: bucket.FirstSeen,
				LastSeen:  bucket.LastSeen,
				Sources:   bucket.Sources,
			})
		}
	}
	return summaries
}

// Evict removes patterns and logs older than windowSec from the given reference time.
func (b *LogBuffer) Evict(refTime int64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	cutoff := refTime - b.windowSec

	// Evict old patterns
	for sig, bucket := range b.patternBuckets {
		if bucket.LastSeen < cutoff {
			delete(b.patternBuckets, sig)
		}
	}

	// Evict old error logs
	newErrors := b.errorLogs[:0]
	for _, log := range b.errorLogs {
		if log.Timestamp >= cutoff {
			newErrors = append(newErrors, log)
		}
	}
	b.errorLogs = newErrors
}

// Stats returns buffer statistics.
func (b *LogBuffer) Stats() LogBufferStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	totalCount := 0
	for _, bucket := range b.patternBuckets {
		totalCount += bucket.Count
	}

	return LogBufferStats{
		UniquePatterns: len(b.patternBuckets),
		ErrorLogCount:  len(b.errorLogs),
		TotalLogsSeen:  totalCount,
		MaxPatterns:    b.maxPatterns,
		MaxErrors:      b.maxErrors,
	}
}

// LogBufferStats contains buffer statistics.
type LogBufferStats struct {
	UniquePatterns int `json:"uniquePatterns"`
	ErrorLogCount  int `json:"errorLogCount"`
	TotalLogsSeen  int `json:"totalLogsSeen"`
	MaxPatterns    int `json:"maxPatterns"`
	MaxErrors      int `json:"maxErrors"`
}

// Clear resets the buffer.
func (b *LogBuffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.patternBuckets = make(map[string]*PatternBucket)
	b.errorLogs = b.errorLogs[:0]
}

// isErrorLevel returns true if the level indicates an error or warning.
func isErrorLevel(level string) bool {
	switch level {
	case "error", "err", "fatal", "critical", "warn", "warning":
		return true
	default:
		return false
	}
}

// containsString checks if a string slice contains a value.
func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// getLogSignature wraps the existing logSignature function for string content.
func getLogSignature(content string) string {
	// Use the existing logSignature from log_timeseries_analysis.go
	// which handles pattern normalization (numbers, UUIDs, etc.)
	return logSignature([]byte(content), 0)
}
