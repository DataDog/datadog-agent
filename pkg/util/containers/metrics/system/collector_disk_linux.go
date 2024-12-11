// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package system

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/provider"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	diskMappingCacheExpiration = time.Minute
)

var diskMappingCacheKey = cache.BuildAgentKey("containers", "disk_mapping")

func buildIOStats(procPath string, cgs *cgroups.IOStats) *provider.ContainerIOStats {
	if cgs == nil {
		return nil
	}
	cs := &provider.ContainerIOStats{}

	convertField(cgs.ReadBytes, &cs.ReadBytes)
	convertField(cgs.WriteBytes, &cs.WriteBytes)
	convertField(cgs.ReadOperations, &cs.ReadOperations)
	convertField(cgs.WriteOperations, &cs.WriteOperations)
	convertFieldAndUnit(cgs.PSISome.Total, &cs.PartialStallTime, float64(time.Microsecond))

	deviceMapping, err := GetDiskDeviceMapping(procPath)
	if err != nil {
		log.Debugf("Error while getting disk mapping, no disk metrics will be present, err:  %v", err)
		return cs
	}

	csDevicesStats := make(map[string]provider.DeviceIOStats, len(cgs.Devices))
	for deviceID, deviceStats := range cgs.Devices {
		if deviceName, found := deviceMapping[deviceID]; found {
			targetDeviceStats := provider.DeviceIOStats{}
			convertField(deviceStats.ReadBytes, &targetDeviceStats.ReadBytes)
			convertField(deviceStats.ReadBytes, &targetDeviceStats.ReadBytes)
			convertField(deviceStats.WriteBytes, &targetDeviceStats.WriteBytes)
			convertField(deviceStats.ReadOperations, &targetDeviceStats.ReadOperations)
			convertField(deviceStats.WriteOperations, &targetDeviceStats.WriteOperations)

			csDevicesStats[deviceName] = targetDeviceStats
		}
	}

	if len(csDevicesStats) > 0 {
		cs.Devices = csDevicesStats
	}

	return cs
}

// GetDiskDeviceMapping scrapes /proc/diskstats to build a mapping from
// "major:minor" device numbers to device name.
// It is cached for 1 minute
// Format:
// 7       0 loop0 0 0 0 0 0 0 0 0 0 0 0
// 7       1 loop1 0 0 0 0 0 0 0 0 0 0 0
// 8       0 sda 24398 2788 1317975 40488 25201 46267 1584744 142336 0 22352 182660
// 8       1 sda1 24232 2788 1312025 40376 25201 46267 1584744 142336 0 22320 182552
// 8      16 sdb 189 0 4063 220 0 0 0 0 0 112 204
func GetDiskDeviceMapping(procPath string) (map[string]string, error) {
	// Cache lookup
	if cached, hit := cache.Cache.Get(diskMappingCacheKey); hit {
		if mapping, ok := cached.(map[string]string); ok {
			return mapping, nil
		}
	}

	// Cache miss, parse file
	statfile := filepath.Join(procPath, "diskstats")
	f, err := os.Open(statfile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	mapping := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 14 {
			// malformed line in /proc/diskstats, avoid panic by ignoring.
			log.Debugf("Malformed line in %s, fields: %v", statfile, fields)
			continue
		}
		mapping[fmt.Sprintf("%s:%s", fields[0], fields[1])] = fields[2]
	}
	if err := scanner.Err(); err != nil {
		log.Debugf("Error while reading %s, disk metrics may be missing, err: %v", statfile, err)
		return mapping, nil
	}

	// Keep value in cache
	cache.Cache.Set(diskMappingCacheKey, mapping, diskMappingCacheExpiration)
	return mapping, nil
}
