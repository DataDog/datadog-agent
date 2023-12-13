// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package limiter

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
)

func getCgroupMemoryLimit() (uint64, error) {
	selfReader, err := cgroups.NewSelfReader("/proc", config.IsContainerized())
	if err != nil {
		return 0, err
	}
	cgroup := selfReader.GetCgroup(cgroups.SelfCgroupIdentifier)
	if cgroup == nil {
		return 0, errors.New("cannot get cgroup")
	}
	var stats cgroups.MemoryStats
	if err := cgroup.GetMemoryStats(&stats); err != nil {
		return 0, err
	}
	if stats.Limit == nil || *stats.Limit == 0 {
		return 0, errors.New("cannot get cgroup memory limit")
	}

	return *stats.Limit, nil
}
