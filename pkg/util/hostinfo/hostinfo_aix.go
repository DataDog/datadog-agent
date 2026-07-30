// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostinfo

import (
	"github.com/shirou/gopsutil/v4/host"

	"github.com/DataDog/datadog-agent/pkg/gohai/platform"
	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetInformation returns host info with PlatformVersion set to "<X>.<Y> TL<Z>"
// format (e.g. "7.3 TL2") derived from oslevel -s output, because oslevel
// (no flags) reports the base install level ("7.3.0.0") regardless of the
// installed Technology Level.
func GetInformation() *host.InfoStat {
	info, _ := cache.Get[*host.InfoStat](
		hostInfoCacheKey,
		func() (*host.InfoStat, error) {
			info, err := host.Info()
			if err != nil {
				log.Errorf("failed to retrieve host info: %s", err)
				return &host.InfoStat{}, err
			}
			// PlatformVersion is formatted as "<X>.<Y> TL<Z>" (e.g. "7.3 TL2"),
			// SP is not included. Derived from oslevel -s output (e.g. "7300-02-02-2419").
			if aixVersion, ok := platform.ParseAIXVersion(info.KernelVersion); ok {
				info.PlatformVersion = aixVersion.PlatformVersion()
			}
			return info, err
		})
	return info
}
