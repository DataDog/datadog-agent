// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && go1.19

package runtime

import (
	"errors"
	"os"
	"runtime/debug"

	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SetGoMemLimit configures Go memory soft limit based on cgroups.
// The soft limit is set to 90% of the cgroup memory hard limit.
// The function is noop if
//   - GOMEMLIMIT is set already
//   - There is no cgroup limit
//
// Read more about Go memory limit in https://tip.golang.org/doc/gc-guide#Memory_limit
func SetGoMemLimit(isContainerized bool) (int64, error) {
	if _, ok := os.LookupEnv("GOMEMLIMIT"); ok {
		log.Debug("GOMEMLIMIT is set already, doing nothing")
		return 0, nil
	}
	selfReader, err := cgroups.NewSelfReader("/proc", isContainerized)
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
	if stats.Limit == nil {
		log.Info("Cgroup memory limit not found, doing nothing")
		return 0, nil
	}
	softLimit := int64(0.8 * float64(*stats.Limit))
	log.Infof("Cgroup memory limit is %d, setting gomemlimit to %d", *stats.Limit, softLimit)
	debug.SetMemoryLimit(softLimit)
	return softLimit, nil
}
