// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package hostinfo

import (
	"github.com/shirou/gopsutil/v4/host"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetInformation returns an InfoStat object with host metadata.
// On AIX, fields that cannot be retrieved are left empty; the function
// never returns a nil pointer and never propagates errors so the result
// is always cached.
func GetInformation() *host.InfoStat {
	info, _ := cache.Get[*host.InfoStat](
		hostInfoCacheKey,
		func() (*host.InfoStat, error) {
			info, err := host.Info()
			if err != nil {
				log.Warnf("failed to retrieve host info on AIX, using partial data: %s", err)
				if info == nil {
					info = &host.InfoStat{}
				}
			}
			// Never return an error so the (possibly partial) result is cached.
			return info, nil
		})
	return info
}
