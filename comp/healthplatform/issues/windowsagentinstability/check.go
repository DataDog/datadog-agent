// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package windowsagentinstability

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
)

const (
	// crashThreshold is the number of crashes in the time window that triggers an issue
	crashThreshold = 2

	// timeWindow is the duration to look back for crash events
	timeWindow = 24 * time.Hour

	// agentLogRelPath is the path to the agent log file relative to ProgramData
	agentLogRelPath = `Datadog\logs\agent.log`
)

// Check scans the Datadog Agent log file for recent crash/exit entries.
// If more than crashThreshold exits are found in the last timeWindow, it returns an IssueReport.
// If the log file is inaccessible or unreadable, the function returns nil to avoid false positives.
func Check() (*healthplatform.IssueReport, error) {
	logPath := resolveAgentLogPath()

	count, err := countRecentCrashes(logPath, timeWindow)
	if err != nil {
		// Graceful degradation: if we can't read the log, don't report a false positive
		return nil, nil //nolint:nilerr
	}

	if count <= crashThreshold {
		return nil, nil
	}

	return &healthplatform.IssueReport{
		IssueId: IssueID,
		Context: map[string]string{
			"crashCount": fmt.Sprintf("%d", count),
			"timeWindow": "24h",
		},
		Tags: []string{"windows", "service-crash", "stability"},
	}, nil
}

// resolveAgentLogPath returns the path to the Datadog Agent log file.
// It uses the PROGRAMDATA environment variable with a fallback to the default location.
func resolveAgentLogPath() string {
	programData := os.Getenv("PROGRAMDATA")
	if programData == "" {
		programData = `C:\ProgramData`
	}
	return filepath.Join(programData, agentLogRelPath)
}

// countRecentCrashes reads the agent log file and counts lines that indicate the agent
// exited unexpectedly within the given time window.
func countRecentCrashes(logPath string, window time.Duration) (int, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	cutoff := time.Now().Add(-window)
	count := 0

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if isCrashLine(line) {
			ts, ok := parseLogTimestamp(line)
			if ok && ts.After(cutoff) {
				count++
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return 0, err
	}

	return count, nil
}

// isCrashLine returns true if the log line indicates an unexpected agent exit or crash.
func isCrashLine(line string) bool {
	lower := strings.ToLower(line)
	return strings.Contains(lower, "agent exited") ||
		strings.Contains(lower, "unexpected exit") ||
		strings.Contains(lower, "panic:") ||
		strings.Contains(lower, "fatal error:")
}

// parseLogTimestamp attempts to parse a timestamp from the beginning of a log line.
// Datadog Agent log lines begin with a date/time prefix, e.g.:
//
//	2024-01-15 10:30:00 UTC | ...
func parseLogTimestamp(line string) (time.Time, bool) {
	// Try common Datadog Agent log timestamp formats
	formats := []string{
		"2006-01-02 15:04:05 MST",
		"2006-01-02 15:04:05 UTC",
		"2006-01-02T15:04:05Z07:00",
	}

	for _, format := range formats {
		prefixLen := len(format)
		if len(line) < prefixLen {
			continue
		}
		ts, err := time.Parse(format, line[:prefixLen])
		if err == nil {
			return ts, true
		}
	}

	return time.Time{}, false
}
