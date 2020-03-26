// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package cgroup

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NanoToUserHZDivisor holds the divisor to convert cpu.usage to the
// same unit as cpu.system (USER_HZ = 1/100)
// TODO: get USER_HZ from gopsutil? Needs to patch it
const NanoToUserHZDivisor float64 = 1e9 / 100

// Mem returns the memory statistics for a Cgroup. If the cgroup file is not
// available then we return an empty stats file.
func (c ContainerCgroup) Mem() (*metrics.ContainerMemStats, error) {
	ret := &metrics.ContainerMemStats{}
	statfile := c.cgroupFilePath("memory", "memory.stat")

	f, err := os.Open(statfile)
	if os.IsNotExist(err) {
		log.Debugf("Missing cgroup file: %s", statfile)
		return ret, nil
	} else if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), " ")
		v, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			continue
		}
		switch fields[0] {
		case "cache":
			ret.Cache = v
		case "swap":
			ret.Swap = v
			ret.SwapPresent = true
		case "rss":
			ret.RSS = v
		case "rss_huge":
			ret.RSSHuge = v
		case "mapped_file":
			ret.MappedFile = v
		case "pgpgin":
			ret.Pgpgin = v
		case "pgpgout":
			ret.Pgpgout = v
		case "pgfault":
			ret.Pgfault = v
		case "pgmajfault":
			ret.Pgmajfault = v
		case "inactive_anon":
			ret.InactiveAnon = v
		case "active_anon":
			ret.ActiveAnon = v
		case "inactive_file":
			ret.InactiveFile = v
		case "active_file":
			ret.ActiveFile = v
		case "unevictable":
			ret.Unevictable = v
		case "hierarchical_memory_limit":
			ret.HierarchicalMemoryLimit = v
		case "hierarchical_memsw_limit":
			ret.HierarchicalMemSWLimit = v
		case "total_cache":
			ret.TotalCache = v
		case "total_rss":
			ret.TotalRSS = v
		case "total_rssHuge":
			ret.TotalRSSHuge = v
		case "total_mapped_file":
			ret.TotalMappedFile = v
		case "total_pgpgin":
			ret.TotalPgpgIn = v
		case "total_pgpgout":
			ret.TotalPgpgOut = v
		case "total_pgfault":
			ret.TotalPgFault = v
		case "total_pgmajfault":
			ret.TotalPgMajFault = v
		case "total_inactive_anon":
			ret.TotalInactiveAnon = v
		case "total_active_anon":
			ret.TotalActiveAnon = v
		case "total_inactive_file":
			ret.TotalInactiveFile = v
		case "total_active_file":
			ret.TotalActiveFile = v
		case "total_unevictable":
			ret.TotalUnevictable = v
		}
	}
	if err := scanner.Err(); err != nil {
		return ret, fmt.Errorf("error reading %s: %s", statfile, err)
	}
	return ret, nil
}

// MemLimit returns the memory limit of the cgroup, if it exists. If the file does not
// exist or there is no limit then this will default to 0.
func (c ContainerCgroup) MemLimit() (uint64, error) {
	v, err := c.ParseSingleStat("memory", "memory.limit_in_bytes")
	if os.IsNotExist(err) {
		log.Debugf("Missing cgroup file: %s",
			c.cgroupFilePath("memory", "memory.limit_in_bytes"))
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	// limit_in_bytes is a special case here, it's possible that it shows a ridiculous number,
	// in which case it represents unlimited, so return 0 here
	if v > uint64(math.Pow(2, 60)) {
		v = 0
	}
	return v, nil
}

// FailedMemoryCount returns the number of times this cgroup reached its memory limit, if it exists.
// If the file does not exist or there is no limit, then this will default to 0
func (c ContainerCgroup) FailedMemoryCount() (uint64, error) {
	v, err := c.ParseSingleStat("memory", "memory.failcnt")
	if os.IsNotExist(err) {
		log.Debugf("Missing cgroup file: %s",
			c.cgroupFilePath("memory", "memory.failcnt"))
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	return v, nil
}

// KernelMemoryUsage returns the number of bytes of kernel memory used by this cgroup, if it exists.
// If the file does not exist or there is an error, then this will default to 0
func (c ContainerCgroup) KernelMemoryUsage() (uint64, error) {
	v, err := c.ParseSingleStat("memory", "memory.kmem.usage_in_bytes")
	if os.IsNotExist(err) {
		log.Debugf("Missing cgroup file: %s",
			c.cgroupFilePath("memory", "memory.kmem.usage_in_bytes"))
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	return v, nil
}

// SoftMemLimit returns the soft memory limit of the cgroup, if it exists. If the file does not
// exist or there is no limit then this will default to 0.
func (c ContainerCgroup) SoftMemLimit() (uint64, error) {
	v, err := c.ParseSingleStat("memory", "memory.soft_limit_in_bytes")
	if os.IsNotExist(err) {
		log.Debugf("Missing cgroup file: %s",
			c.cgroupFilePath("memory", "memory.soft_limit_in_bytes"))
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	// limit_in_bytes is a special case here, it's possible that it shows a ridiculous number,
	// in which case it represents unlimited, so return 0 here
	if v > uint64(math.Pow(2, 60)) {
		v = 0
	}
	return v, nil
}

// CPU returns the CPU status for this cgroup instance
// If the cgroup file does not exist then we just log debug return nothing.
func (c ContainerCgroup) CPU() (*metrics.ContainerCPUStats, error) {
	ret := &metrics.ContainerCPUStats{}
	statfile := c.cgroupFilePath("cpuacct", "cpuacct.stat")
	f, err := os.Open(statfile)
	if os.IsNotExist(err) {
		log.Debugf("Missing cgroup file: %s", statfile)
		return ret, nil
	} else if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), " ")
		if fields[0] == "user" {
			user, err := strconv.ParseUint(fields[1], 10, 64)
			if err == nil {
				ret.User = user
			}
		}
		if fields[0] == "system" {
			system, err := strconv.ParseUint(fields[1], 10, 64)
			if err == nil {
				ret.System = system
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ret, fmt.Errorf("error reading %s: %s", statfile, err)
	}

	usage, err := c.ParseSingleStat("cpuacct", "cpuacct.usage")
	if err == nil {
		ret.UsageTotal = float64(usage) / NanoToUserHZDivisor
	} else {
		log.Debugf("Missing total cpu usage stat for %s: %s", c.ContainerID, err.Error())
	}

	shares, err := c.ParseSingleStat("cpu", "cpu.shares")
	if err == nil {
		ret.Shares = shares
	} else {
		log.Debugf("Missing cpu shares stat for %s: %s", c.ContainerID, err.Error())
	}

	return ret, nil
}

// CPUNrThrottled returns the number of times the cgroup has been
// throttle/limited because of CPU quota / limit
// If the cgroup file does not exist then we just log debug and return 0.
func (c ContainerCgroup) CPUNrThrottled() (uint64, error) {
	statfile := c.cgroupFilePath("cpu", "cpu.stat")
	f, err := os.Open(statfile)
	if os.IsNotExist(err) {
		log.Debugf("Missing cgroup file: %s", statfile)
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), " ")
		if fields[0] == "nr_throttled" {
			value, err := strconv.ParseUint(fields[1], 10, 64)
			if err != nil {
				return 0, err
			}
			return value, nil
		}
	}
	log.Debugf("Missing nr_throttled line in %s", statfile)
	return 0, nil
}

// CPULimit would show CPU limit for this cgroup.
// It does so by checking the cpu period and cpu quota config
// if a user does this:
//
//	docker run --cpus='0.5' ubuntu:latest
//
// we should return 50% for that container
// If the limits files aren't available (on older version) then
// we'll return the default value of 100.
func (c ContainerCgroup) CPULimit() (float64, error) {
	periodFile := c.cgroupFilePath("cpu", "cpu.cfs_period_us")
	quotaFile := c.cgroupFilePath("cpu", "cpu.cfs_quota_us")
	plines, err := readLines(periodFile)
	if os.IsNotExist(err) {
		log.Debugf("Missing cgroup file: %s", periodFile)
		return 100, nil
	} else if err != nil {
		return 0, err
	}
	qlines, err := readLines(quotaFile)
	if os.IsNotExist(err) {
		log.Debugf("Missing cgroup file: %s", quotaFile)
		return 100, nil
	} else if err != nil {
		return 0, err
	}
	period, err := strconv.ParseFloat(plines[0], 64)
	if err != nil {
		return 0, err
	}
	quota, err := strconv.ParseFloat(qlines[0], 64)
	if err != nil {
		return 0, err
	}
	// default cpu limit is 100%
	limit := 100.0
	if (period > 0) && (quota > 0) {
		limit = (quota / period) * 100.0
	}
	return limit, nil
}

// IO returns the disk read and write bytes stats for this cgroup.
// tested in DiskMappingTestSuite.TestContainerCgroupIO
// Format:
//
// 8:0 Read 49225728
// 8:0 Write 9850880
// 8:0 Sync 0
// 8:0 Async 59076608
// 8:0 Total 59076608
// 252:0 Read 49094656
// 252:0 Write 9850880
// 252:0 Sync 0
// 252:0 Async 58945536
// 252:0 Total 58945536
//
func (c ContainerCgroup) IO() (*metrics.ContainerIOStats, error) {
	ret := &metrics.ContainerIOStats{
		DeviceReadBytes:  make(map[string]uint64),
		DeviceWriteBytes: make(map[string]uint64),
	}

	statfile := c.cgroupFilePath("blkio", "blkio.throttle.io_service_bytes")
	f, err := os.Open(statfile)
	if os.IsNotExist(err) {
		log.Debugf("Missing cgroup file: %s", statfile)
		return ret, nil
	} else if err != nil {
		return nil, err
	}
	defer f.Close()

	// Get device id->name mapping
	var devices map[string]string
	mapping, err := getDiskDeviceMapping()
	if err != nil {
		log.Debugf("Cannot get per-device stats: %s", err)
		// devices will stay nil, lookups are safe in nil maps
	} else {
		devices = mapping.idToName
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), " ")
		if len(fields) < 3 {
			continue
		}
		deviceName := devices[fields[0]]
		if fields[1] == "Read" {
			read, err := strconv.ParseUint(fields[2], 10, 64)
			if err == nil {
				ret.ReadBytes += read
				if deviceName != "" {
					ret.DeviceReadBytes[deviceName] = read
				}
			}
		} else if fields[1] == "Write" {
			write, err := strconv.ParseUint(fields[2], 10, 64)
			if err == nil {
				ret.WriteBytes += write
				if deviceName != "" {
					ret.DeviceWriteBytes[deviceName] = write
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ret, fmt.Errorf("error reading %s: %s", statfile, err)
	}

	var fileDescCount uint64
	for _, pid := range c.Pids {
		fdCount, err := GetFileDescriptorLen(int(pid))
		if err != nil {
			log.Warnf("Failed to get file desc length for pid %d, container %s: %s", pid, c.ContainerID[:12], err)
			continue
		}
		fileDescCount += uint64(fdCount)
	}
	ret.OpenFiles = fileDescCount

	return ret, nil
}

// ThreadCount returns the number of threads in the pid cgroup
// linked to the container.
// ref: https://www.kernel.org/doc/Documentation/cgroup-v1/pids.txt
//
// Although the metric is called `pid.current`, it also tracks
// threads, and not only task-group-pids
func (c ContainerCgroup) ThreadCount() (uint64, error) {
	v, err := c.ParseSingleStat("pids", "pids.current")
	if os.IsNotExist(err) {
		log.Debugf("Missing cgroup file: %s",
			c.cgroupFilePath("pids", "pids.current"))
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	return v, nil
}

// ThreadLimit returns the thread count limit in the pid cgroup
// linked to the container.
// ref: https://www.kernel.org/doc/Documentation/cgroup-v1/pids.txt
//
// If `max` is found, the method returns 0 as-in "no limit"
func (c ContainerCgroup) ThreadLimit() (uint64, error) {
	statFile := c.cgroupFilePath("pids", "pids.max")
	lines, err := readLines(statFile)
	if os.IsNotExist(err) {
		log.Debugf("Missing cgroup file: %s", statFile)
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	if len(lines) != 1 {
		return 0, fmt.Errorf("wrong file format: %s", statFile)
	}
	if lines[0] == "max" {
		return 0, nil
	}
	value, err := strconv.ParseUint(lines[0], 10, 64)
	if err != nil {
		return 0, err
	}
	return value, nil
}

// ParseSingleStat reads and converts a single-value cgroup stat file content to uint64.
func (c ContainerCgroup) ParseSingleStat(target, file string) (uint64, error) {
	statFile := c.cgroupFilePath(target, file)
	lines, err := readLines(statFile)
	if err != nil {
		return 0, err
	}
	if len(lines) != 1 {
		return 0, fmt.Errorf("wrong file format: %s", statFile)
	}
	value, err := strconv.ParseUint(lines[0], 10, 64)
	if err != nil {
		return 0, err
	}
	return value, nil
}
