// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package enhancedmetrics

import (
	"errors"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	systemutils "github.com/DataDog/datadog-agent/pkg/util/system"
)

type cgroupCPUStatsProvider struct {
	reader CgroupReader
}

func (p *cgroupCPUStatsProvider) read(collectionTime time.Time) (*ServerlessContainerStats, error) {
	if err := p.reader.RefreshCgroups(0); err != nil {
		return nil, fmt.Errorf("failed to refresh cgroups: %w", err)
	}

	cgroup := p.reader.GetCgroup(cgroups.SelfCgroupIdentifier)
	if cgroup == nil {
		return nil, errors.New("failed to get self cgroup")
	}

	stats := &cgroups.Stats{}
	allFailed, errs := cgroups.GetStats(cgroup, stats)
	if allFailed {
		return nil, fmt.Errorf("failed to get cgroup stats: %v", errs)
	}
	if len(errs) > 0 {
		log.Debugf("Incomplete cgroup stats: %v", errs)
	}

	return convertCgroupStats(stats, collectionTime), nil
}

func newCPUStatsProvider(metricSource metrics.MetricSource, reader CgroupReader, cgroupErr error) (cpuStatsProvider, error) {
	if cgroupErr == nil {
		return &cgroupCPUStatsProvider{reader: reader}, nil
	}
	if metricSource == metrics.MetricSourceAWSMicroVMEnhanced && errors.Is(cgroupErr, cgroups.ErrNoCgroupMount) {
		return newProcStatCPUStatsProvider("/proc", systemutils.HostCPUCount()), nil
	}
	return nil, cgroupErr
}
