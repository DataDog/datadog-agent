// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//
// Package hostinfo helps collect relevant host information
//
//
// # Compatibility
//
// This module is exported and can be used outside of the datadog-agent
// repository, but is not designed as a general-purpose logging system.  Its
// API may change incompatibly.

//go:build !windows

package hostinfo

import (
	"github.com/shirou/gopsutil/v4/host"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetInformation returns an InfoStat object, filled in with various operating system metadata. This returns an empty
// host.InfoStat if gopsutil fails.
func GetInformation() *host.InfoStat {
	info, _ := cache.Get[*host.InfoStat](
		hostInfoCacheKey,
		func() (*host.InfoStat, error) {
			info, err := host.Info()
			if err != nil {
				log.Errorf("failed to retrieve host info: %s", err)
				return &host.InfoStat{}, err
			}
			return info, err
		})
	return info
}
