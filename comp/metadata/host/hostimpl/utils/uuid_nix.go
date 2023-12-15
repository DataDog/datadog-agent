// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package utils

import (
	gopsutilhost "github.com/shirou/gopsutil/v3/host"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// GetUUID returns the host ID.
func getUUID() string {
	guid, _ := cache.Get[string](
		guidCacheKey,
		func() (string, error) {
			info, err := gopsutilhost.Info()
			if err != nil {
				return "", log.Errorf("failed to retrieve host info: %s", err)
			}
			return info.HostID, nil
		})
	return guid
}
