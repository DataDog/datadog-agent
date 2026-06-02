// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

package servicemain

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// CrashCounterRelPath is the path to the crash counter file relative to PROGRAMDATA.
	CrashCounterRelPath = `Datadog\run\service-crashes.log`
)

func crashCounterPath() string {
	pd := os.Getenv("PROGRAMDATA")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	return filepath.Join(pd, CrashCounterRelPath)
}

// WriteCrashEntry appends the current Unix timestamp to the crash counter file.
// It is called by servicemain at each unclean service exit so that the next
// startup can count recent failures without re-parsing agent logs.
func WriteCrashEntry() error {
	path := crashCounterPath()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%d\n", time.Now().Unix())
	return err
}

// ReadRecentCrashCount returns the number of crash entries recorded within window.
// Returns 0, nil when the counter file does not exist (no recorded crashes).
func ReadRecentCrashCount(window time.Duration) (int, error) {
	path := crashCounterPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}
	cutoff := time.Now().Add(-window).Unix()
	count := 0
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		ts, parseErr := strconv.ParseInt(line, 10, 64)
		if parseErr != nil {
			continue
		}
		if ts > cutoff {
			count++
		}
	}
	return count, nil
}
