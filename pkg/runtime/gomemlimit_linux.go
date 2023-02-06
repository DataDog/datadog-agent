// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package runtime

import (
	"context"
	"errors"
	"math"
	"runtime/debug"
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var memoryLimitTelemetry = telemetry.NewGauge("limiter", "mem_limit", nil, "The current value of GOMEMLIMIT")

type staticMemoryLimiter struct {
	customLimit     uint64
	limitPct        float64
	isContainerized bool
}

// NewStaticMemoryLimiter return a memory limit that sets memory limit once based on cgroup limit
func NewStaticMemoryLimiter(limitPct float64, customLimit uint64, isContainerized bool) MemoryLimiter {
	return staticMemoryLimiter{
		customLimit:     customLimit,
		limitPct:        limitPct,
		isContainerized: isContainerized,
	}
}

// Run runs the memory limit logic
func (s staticMemoryLimiter) Run(context.Context) error {
	// SetMemoryLimit with negative values returns current value
	if debug.SetMemoryLimit(-1) != math.MaxInt64 {
		log.Debug("GOMEMLIMIT is set already, doing nothing")
		return nil
	}

	if s.customLimit > 0 {
		s.setMemoryLimit(s.customLimit)
	}

	selfCgroup, err := getSelfCgroup(s.isContainerized)
	if err != nil {
		return err
	}
	var stats cgroups.MemoryStats
	if err := selfCgroup.GetMemoryStats(&stats); err != nil {
		return err
	}
	if stats.Limit == nil {
		return nil
	}

	s.setMemoryLimit(*stats.Limit)
	return nil
}

func (s staticMemoryLimiter) setMemoryLimit(cgroupLimit uint64) {
	memLimit := int64(s.limitPct * float64(cgroupLimit))
	log.Infof("Cgroup memory limit is %d, setting gomemlimit to %d", cgroupLimit, memLimit)
	memoryLimitTelemetry.Set(float64(memLimit))
	debug.SetMemoryLimit(memLimit)
}

type dynamicMemoryLimiter struct {
	ticker               *time.Ticker
	selfCgroup           cgroups.Cgroup
	minLimitPct          float64
	externalMemoryReader func(cgroups.MemoryStats) uint64
}

// NewDynamicMemoryLimiter creates a memory limiter that adapts limit continuously based on Cgroup limit
// plus memory consumed outside of the Go space.
// It will set GOMEMLIMT to max(cgroupLimit * minLimitPct, cgroupLimit-externalMemory)
// It will override existing value of GOMEMLIMIT
func NewDynamicMemoryLimiter(interval time.Duration, isContainerized bool, minLimitPct float64, externalMemoryReader func(cgroups.MemoryStats) uint64) (MemoryLimiter, error) {
	if externalMemoryReader == nil {
		return nil, errors.New("unable to create dynamicMemoryLimiter, externalMemoryReader function is missing")
	}

	if debug.SetMemoryLimit(-1) != math.MaxInt64 {
		return nil, errors.New(("GOMEMLIMIT is set already, doing nothing"))
	}

	selfCgroup, err := getSelfCgroup(isContainerized)
	if err != nil {
		return nil, err
	}

	return dynamicMemoryLimiter{
		ticker:               time.NewTicker(interval),
		selfCgroup:           selfCgroup,
		externalMemoryReader: externalMemoryReader,
	}, nil
}

// Run runs the memory limit logic
func (d dynamicMemoryLimiter) Run(c context.Context) error {
	log.Info("Starting dynamic memory limiter")

	for {
		select {
		case <-c.Done():
			return nil

		case <-d.ticker.C:
			d.computeSetLimit()
		}
	}
}

func (d dynamicMemoryLimiter) computeSetLimit() {
	var cgroupMemStats cgroups.MemoryStats
	if err := d.selfCgroup.GetMemoryStats(&cgroupMemStats); err != nil {
		log.Warnf("Unable to get self memory stats, dynamic memory limiter unavailable, err: %v", err)
		return
	}

	// If no limit is set, we'll let the system OOM do its job in case of system-wide memory shortage.
	if cgroupMemStats.Limit == nil {
		return
	}

	externalMemory := d.externalMemoryReader(cgroupMemStats)
	goMemorySpace := *cgroupMemStats.Limit - externalMemory
	minGoMemorySpace := uint64(float64(*cgroupMemStats.Limit) * d.minLimitPct)
	if goMemorySpace < minGoMemorySpace {
		goMemorySpace = minGoMemorySpace
	}

	log.Tracef("Setting Go memory limit to: %d, external memory: %d", goMemorySpace, externalMemory)
	memoryLimitTelemetry.Set(float64(goMemorySpace))
	debug.SetMemoryLimit(int64(goMemorySpace))
}

func getSelfCgroup(isContainerized bool) (cgroups.Cgroup, error) {
	cgroupReader, err := cgroups.NewSelfReader("/proc", isContainerized)
	if err != nil {
		return nil, err
	}

	selfCgroup := cgroupReader.GetCgroup(cgroups.SelfCgroupIdentifier)
	if selfCgroup == nil {
		return nil, errors.New("cannot get self cgroup")
	}

	return selfCgroup, nil
}
