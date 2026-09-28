// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package enhancedmetrics

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	gopsutilcommon "github.com/shirou/gopsutil/v4/common"
	"github.com/shirou/gopsutil/v4/cpu"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const maxUint64 = ^uint64(0)

type procStatCPUStatsProvider struct {
	procPath    string
	cpuCount    int
	getCPUTimes func(context.Context, bool) ([]cpu.TimesStat, error)
}

func newProcStatCPUStatsProvider(procPath string, cpuCount int) *procStatCPUStatsProvider {
	return &procStatCPUStatsProvider{
		procPath:    procPath,
		cpuCount:    cpuCount,
		getCPUTimes: cpu.TimesWithContext,
	}
}

func (p *procStatCPUStatsProvider) read(collectionTime time.Time) (*ServerlessContainerStats, error) {
	ctx := context.WithValue(
		context.Background(),
		gopsutilcommon.EnvKey,
		gopsutilcommon.EnvMap{gopsutilcommon.HostProcEnvKey: p.procPath},
	)
	times, err := p.getCPUTimes(ctx, false)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s/stat: %w", p.procPath, err)
	}
	if len(times) == 0 {
		return nil, fmt.Errorf("failed to read %s/stat: no aggregate CPU stats returned", p.procPath)
	}

	total, err := cpuTimesToNanoseconds(times[0])
	if err != nil {
		return nil, fmt.Errorf("failed to convert CPU stats from %s/stat: %w", p.procPath, err)
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

// cpuTimesToNanoseconds returns aggregate guest CPU busy time in nanoseconds.
// User, nice, system, irq, and softirq are counted; idle, iowait, and steal
// are excluded because they are not time executing guest work. Guest and
// guest_nice are not added because Linux already includes them in user and
// nice, respectively.
func cpuTimesToNanoseconds(stats cpu.TimesStat) (uint64, error) {
	busyNanoseconds := (stats.User + stats.Nice + stats.System + stats.Irq + stats.Softirq) * float64(time.Second)
	if math.IsNaN(busyNanoseconds) || math.IsInf(busyNanoseconds, 0) || busyNanoseconds < 0 {
		return 0, errors.New("invalid aggregate CPU time")
	}

	busyNanoseconds = math.Round(busyNanoseconds)
	if busyNanoseconds >= float64(maxUint64) {
		return 0, errors.New("aggregate CPU time conversion overflow")
	}

	return uint64(busyNanoseconds), nil
}
