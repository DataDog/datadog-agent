// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package cgroup

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

type diskDeviceMapping struct {
	idToName map[string]string
}

var diskMappingCacheKey = cache.BuildAgentKey("containers", "disk_mapping")

// getDiskDeviceMapping scrapes /proc/diskstats to build a mapping from
// "major:minor" device numbers to device name.
// It is cached for 1 minute
// Format:
// 7       0 loop0 0 0 0 0 0 0 0 0 0 0 0
// 7       1 loop1 0 0 0 0 0 0 0 0 0 0 0
// 8       0 sda 24398 2788 1317975 40488 25201 46267 1584744 142336 0 22352 182660
// 8       1 sda1 24232 2788 1312025 40376 25201 46267 1584744 142336 0 22320 182552
// 8      16 sdb 189 0 4063 220 0 0 0 0 0 112 204
func getDiskDeviceMapping() (*diskDeviceMapping, error) {
	// Cache lookup
	var mapping *diskDeviceMapping
	var ok bool
	if cached, hit := cache.Cache.Get(diskMappingCacheKey); hit {
		if mapping, ok = cached.(*diskDeviceMapping); ok {
			return mapping, nil
		}
	}

	// Cache miss, parse file
	statfile := hostProc("diskstats")
	f, err := os.Open(statfile)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	mapping = &diskDeviceMapping{
		idToName: make(map[string]string),
	}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 14 {
			// malformed line in /proc/diskstats, avoid panic by ignoring.
			continue
		}
		mapping.idToName[fmt.Sprintf("%s:%s", fields[0], fields[1])] = fields[2]
	}
	if err := scanner.Err(); err != nil {
		return mapping, fmt.Errorf("error reading %s: %s", statfile, err)
	}

	// Keep value in cache
	cache.Cache.Set(diskMappingCacheKey, mapping, time.Minute)
	return mapping, nil
}
