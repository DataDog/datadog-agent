// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package oom provides utilities for getting and setting OOM (Out of Memory) score adjustments for processes.
package oom

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
)

// GetOOMScoreAdj returns the OOM score adjustment for the given process, or for the current process if pid is 0.
func GetOOMScoreAdj(pid int) (int, error) {
	pidString := "self"
	if pid != 0 {
		pidString = strconv.Itoa(pid)
	}

	procPath := fmt.Sprintf("/proc/%s/oom_score_adj", pidString)
	data, err := os.ReadFile(procPath)
	if err != nil {
		return -1, fmt.Errorf("failed to read oom_score_adj to %s for PID %d: %w", procPath, pid, err)
	}

	// trim the trailing newline
	data = bytes.TrimSuffix(data, []byte("\n"))
	score, err := strconv.Atoi(string(data))

	if err != nil {
		return -1, fmt.Errorf("oom_score_adj is not a number from %s for PID %d: %w", procPath, pid, err)
	}

	return score, nil
}

// SetOOMScoreAdj sets the OOM score adjustment for the given process, or for the current process if pid is 0.
func SetOOMScoreAdj(pid, score int) error {
	if score < -1000 || score > 1000 {
		return fmt.Errorf("oom_score_adj must be between -1000 and 1000, got %d", score)
	}

	pidString := "self"
	if pid != 0 {
		pidString = strconv.Itoa(pid)
	}

	procPath := fmt.Sprintf("/proc/%s/oom_score_adj", pidString)

	if err := os.WriteFile(procPath, []byte(strconv.Itoa(score)), 0); err != nil {
		return fmt.Errorf("failed to write oom_score_adj to %s for PID %d: %w", procPath, pid, err)
	}

	return nil
}
