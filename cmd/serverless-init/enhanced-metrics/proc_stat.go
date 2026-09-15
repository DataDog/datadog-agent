// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package enhancedmetrics

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	procStatClockTicksPerSecond = 100
	procStatNanosecondsPerTick  = uint64(time.Second) / procStatClockTicksPerSecond
	maxUint64                   = ^uint64(0)
)

type procStatCPUStatsProvider struct {
	path     string
	cpuCount int
	readFile func(string) ([]byte, error)
}

func newProcStatCPUStatsProvider(procPath string, cpuCount int) *procStatCPUStatsProvider {
	return &procStatCPUStatsProvider{
		path:     filepath.Join(procPath, "stat"),
		cpuCount: cpuCount,
		readFile: os.ReadFile,
	}
}

func (p *procStatCPUStatsProvider) read(collectionTime time.Time) (*ServerlessContainerStats, error) {
	data, err := p.readFile(p.path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", p.path, err)
	}

	total, err := parseProcStatCPUTime(data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", p.path, err)
	}

	cpuStats := &ServerlessCPUStats{Total: pointer.Ptr(total)}
	if p.cpuCount > 0 {
		// In an AWS Lambda MicroVM, the guest-visible logical CPU count is the
		// validated CPU entitlement used as the fallback capacity denominator.
		cpuStats.Limit = pointer.Ptr(float64(p.cpuCount) * 1e9)
	}

	return &ServerlessContainerStats{
		CollectionTime: collectionTime,
		CPU:            cpuStats,
	}, nil
}

// parseProcStatCPUTime returns aggregate guest CPU busy time in nanoseconds.
// /proc/stat reports USER_HZ ticks. User, nice, system, irq, and softirq are
// counted; iowait and steal are excluded because they are not time executing
// guest work. Guest and guest_nice are not added because Linux already includes
// them in user and nice, respectively.
func parseProcStatCPUTime(data []byte) (uint64, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 || fields[0] != "cpu" {
			continue
		}
		if len(fields) < 8 {
			return 0, fmt.Errorf("aggregate cpu line has %d fields, want at least 8", len(fields))
		}

		var busyTicks uint64
		for _, index := range []int{1, 2, 3, 6, 7} {
			ticks, err := strconv.ParseUint(fields[index], 10, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid cpu counter %q: %w", fields[index], err)
			}
			if busyTicks > maxUint64-ticks {
				return 0, errors.New("aggregate cpu counter overflow")
			}
			busyTicks += ticks
		}

		if busyTicks > maxUint64/procStatNanosecondsPerTick {
			return 0, errors.New("aggregate cpu time conversion overflow")
		}
		return busyTicks * procStatNanosecondsPerTick, nil
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("aggregate cpu line not found")
}
