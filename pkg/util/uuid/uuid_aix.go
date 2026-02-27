// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package uuid

import (
	gopsutilhost "github.com/shirou/gopsutil/v4/host"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// getUUID returns the host ID.
// On AIX, host.Info() may fail (e.g. if 'bootinfo' is not in PATH), in which
// case an empty string is returned without propagating the error.
func getUUID() string {
	guid, _ := cache.Get[string](
		guidCacheKey,
		func() (string, error) {
			info, err := gopsutilhost.Info()
			if err != nil {
				log.Warnf("failed to retrieve host info on AIX, UUID will be empty: %s", err)
				// Return empty string without an error so the result is cached and
				// we don't retry (and re-log) on every call.
				return "", nil
			}
			return info.HostID, nil
		})
	return guid
}
