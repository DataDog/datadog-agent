// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_logs

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	hostRoot         = "/host"
	maxLineCount     = 1000
	defaultLineCount = 10
	maxLineLength    = 64 * 1024 // 64KB
)

// sanitizeAndResolvePath cleans a user-provided file path, validates it is absolute,
// prepends the /host root, and confirms the result still resides under /host.
func sanitizeAndResolvePath(userPath string) (string, error) {
	cleaned := filepath.Clean(userPath)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("path must be absolute, got: %s", userPath)
	}
	resolved := filepath.Join(hostRoot, cleaned)
	// Re-clean to collapse any traversal introduced by the join.
	resolved = filepath.Clean(resolved)
	if !strings.HasPrefix(resolved, hostRoot+"/") {
		return "", fmt.Errorf("resolved path %q escapes host root", resolved)
	}
	return resolved, nil
}

// clampLineCount constrains the requested line count to [1, maxLineCount],
// defaulting to defaultLineCount when n <= 0.
func clampLineCount(n int) int {
	if n <= 0 {
		return defaultLineCount
	}
	if n > maxLineCount {
		return maxLineCount
	}
	return n
}

// headFile reads the first lineCount lines from the file at filePath.
// Lines longer than maxLineLength are truncated.
func headFile(filePath string, lineCount int) (string, int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, maxLineLength), maxLineLength)

	var lines []string
	for scanner.Scan() {
		if len(lines) >= lineCount {
			break
		}
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", 0, fmt.Errorf("error reading file %s: %w", filePath, err)
	}
	return strings.Join(lines, "\n"), len(lines), nil
}

// tailFile reads the last lineCount lines from the file at filePath using a
// ring buffer so only O(lineCount) memory is used rather than reading the
// entire file into memory.
func tailFile(filePath string, lineCount int) (string, int, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", 0, fmt.Errorf("failed to open file %s: %w", filePath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, maxLineLength), maxLineLength)

	ring := make([]string, lineCount)
	idx := 0
	total := 0
	for scanner.Scan() {
		ring[idx%lineCount] = scanner.Text()
		idx++
		total++
	}
	if err := scanner.Err(); err != nil {
		return "", 0, fmt.Errorf("error reading file %s: %w", filePath, err)
	}

	count := total
	if count > lineCount {
		count = lineCount
	}

	result := make([]string, 0, count)
	start := 0
	if total > lineCount {
		start = idx % lineCount
	}
	for i := 0; i < count; i++ {
		result = append(result, ring[(start+i)%lineCount])
	}
	return strings.Join(result, "\n"), count, nil
}
