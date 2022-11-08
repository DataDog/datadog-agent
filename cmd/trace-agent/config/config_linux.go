// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package config

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	systemutils "github.com/DataDog/datadog-agent/pkg/util/system"
)

func getCgroupCPULimit() (float64, error) {
	reader, err := cgroups.NewSelfReader("/proc", config.IsContainerized())
	if err != nil {
		return 0, err
	}
	cg := reader.GetCgroup(cgroups.SelfCgroupIdentifier)
	if cg == nil {
		return 0, errors.New("cannot get self cgroup")
	}
	var cgs cgroups.CPUStats
	err = cg.GetCPUStats(&cgs)
	if err != nil {
		return 0, err
	}
	// Limit is computed using min(CPUSet, CFS CPU Quota)
	var limitPct *float64

	if cgs.CPUCount != nil && *cgs.CPUCount != uint64(systemutils.HostCPUCount()) {
		limitPct = pointer.UIntToFloatPtr(*cgs.CPUCount * 100)
	}
	if cgs.SchedulerQuota != nil && cgs.SchedulerPeriod != nil {
		quotaLimitPct := 100 * (float64(*cgs.SchedulerQuota) / float64(*cgs.SchedulerPeriod))
		if limitPct == nil || quotaLimitPct < *limitPct {
			limitPct = &quotaLimitPct
		}
	}

	return *limitPct, nil
}
