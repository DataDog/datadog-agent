// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostinfo

import (
	"fmt"
	"strings"

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
			if platformVersion := kernelVersionToPlatformVersion(info.KernelVersion); platformVersion != "" {
				info.PlatformVersion = platformVersion
			}
			return info, err
		})
	return info
}

// kernelVersionToPlatformVersion derives "<X>.<Y> TL<Z>" from an
// kernel version string (e.g. "7.3.1.4" -> "7.3 TL1").
// Supplementary Package (SP) is not included in the platform version.
func kernelVersionToPlatformVersion(osLevelVersion string) string {
	kernelVersion := platform.ParseKernelVersionFromOsLevel(osLevelVersion)
	parts := strings.Split(kernelVersion, ".")
	if len(parts) < 3 {
		return ""
	}
	return fmt.Sprintf("%s.%s TL%s", parts[0], parts[1], parts[2])
}
