// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package cgroups

import (
	"io/ioutil"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/system"

	"github.com/karrick/godirwalk"
)

// ReadCgroupReferences returns all cgroups paths for pid
func ReadCgroupReferences(procPath string, pid string) (string, error) {
	cgPath := filepath.Join(procPath, pid, procCgroupFile)
	content, err := ioutil.ReadFile(cgPath)
	if err != nil {
		return "", err
	}

	return string(content), nil
}

// Unfortunately, the reading of `<host_path>/sys/fs/cgroup/pids/.../cgroup.procs` is PID-namespace aware,
// meaning that we cannot rely on it to find all PIDs belonging to a cgroupp, except if the Agent runs in host PID namespace.
type pidMapper interface {
	getPIDsForCgroup(relativeCgroupPath string, cacheValidity time.Duration) []int
}

// cgroupRoot is cgroup base directory (like /host/sys/fs/cgroup)
func getPidMapper(procPath, cgroupRoot, baseController string, filter ReaderFilter) pidMapper {
	// Checking if we are in host pid. If that's the case `cgroup.procs` in any controller will contain PIDs
	// In cgroupv2, the file contains 0 values, filtering for that
	cgroupProcsTestFilePath := filepath.Join(cgroupRoot, baseController, cgroupProcsFile)
	cgroupProcsUsable := false
	err := parseFile(defaultFileReader, cgroupProcsTestFilePath, func(s string) error {
		if s != "" && s != "0" {
			cgroupProcsUsable = true
		}

		return nil
	})

	if cgroupProcsUsable {
		return &cgroupProcsPidMapper{
			fr: defaultFileReader,
			cgroupProcsFilePathBuilder: func(relativeCgroupPath string) string {
				return filepath.Join(cgroupRoot, baseController, relativeCgroupPath, cgroupProcsFile)
			},
		}
	}
	log.Debugf("cgroup.procs file at: %s is empty or unreadable, considering we're not running in host PID namespace, err: %v", cgroupProcsTestFilePath, err)

	// Checking if we're in host cgroup namespace, other the method below cannot be used either
	// (we'll still return it in case the cgroup namespace detection failed but log a warning)
	pidMapper := &procPidMapper{
		fr:               defaultFileReader,
		readerFilter:     filter,
		procPath:         procPath,
		cgroupController: baseController,
	}

	// In cgroupv2, checking if we run in host cgroup namespace.
	// If not we cannot fill PIDs for containers and do PID<>CID mapping.
	if baseController == "" {
		cgroupInode, err := system.GetProcessNamespaceInode("/proc", "self", "cgroup")
		if err == nil {
			if isHostNs := system.IsProcessHostCgroupNamespace(procPath, cgroupInode); isHostNs != nil && !*isHostNs {
				log.Warnf("Usage of cgroupv2 detected but the Agent does not seem to run in host cgroup namespace. Make sure to run with --cgroupns=host, some feature may not work otherwise")
			}
		} else {
			log.Debugf("Unable to get self cgroup namespace inode, err: %v", err)
		}
	}

	return pidMapper
}

// Mapper used if we are running in host PID namespace, faster.
type cgroupProcsPidMapper struct {
	fr fileReader
	// args are: relative cgroup path
	cgroupProcsFilePathBuilder func(string) string
}

func (pm *cgroupProcsPidMapper) getPIDsForCgroup(relativeCgroupPath string, cacheValidity time.Duration) []int {
	var pids []int

	if err := parseFile(pm.fr, pm.cgroupProcsFilePathBuilder(relativeCgroupPath), func(s string) error {
		pid, err := strconv.Atoi(s)
		if err != nil {
			reportError(newValueError(s, err))
			return nil
		}

		pids = append(pids, pid)

		return nil
	}); err != nil {
		reportError(err)
	}

	return pids
}

// Mapper used if we are NOT running in host PID namespace (most common cases in containers)
type procPidMapper struct {
	lock              sync.Mutex
	refreshTimestamp  time.Time
	fr                fileReader
	readerFilter      ReaderFilter
	procPath          string
	cgroupController  string
	cgroupPidsMapping map[string][]int
}

func (pm *procPidMapper) refreshMapping(cacheValidity time.Duration) {
	if pm.refreshTimestamp.Add(cacheValidity).After(time.Now()) {
		return
	}

	cgroupPidMapping := make(map[string][]int)

	// Going through everything in `<procPath>/<pid>/cgroup`
	err := godirwalk.Walk(pm.procPath, &godirwalk.Options{
		AllowNonDirectory: true,
		Unsorted:          true,
		Callback: func(fullPath string, de *godirwalk.Dirent) error {
			// The callback will be first called with the directory itself
			if de.Name() == "proc" {
				return nil
			}

			pid, err := strconv.ParseInt(de.Name(), 10, 64)
			if err != nil {
				return godirwalk.SkipThis
			}

			_ = parseFile(pm.fr, filepath.Join(fullPath, procCgroupFile), func(s string) error {
				parts := strings.Split(s, ":")
				// Skip potentially malformed lines
				if len(parts) != 3 {
					return nil
				}

				if parts[1] != pm.cgroupController {
					return nil
				}

				// We need to remove first / as the path produced in Readers may not include it
				cgroupRelativePath := strings.TrimLeft(parts[2], "/")
				id, err := pm.readerFilter(cgroupRelativePath, filepath.Base(cgroupRelativePath))
				if id == "" || err != nil {
					return nil
				}

				cgroupPidMapping[cgroupRelativePath] = append(cgroupPidMapping[cgroupRelativePath], int(pid))
				return &stopParsingError{}
			})

			return godirwalk.SkipThis
		},
	})

	pm.refreshTimestamp = time.Now()
	if err != nil {
		reportError(err)
	} else {
		pm.cgroupPidsMapping = cgroupPidMapping
	}
}

func (pm *procPidMapper) getPIDsForCgroup(relativeCgroupPath string, cacheValidity time.Duration) []int {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	pm.refreshMapping(cacheValidity)
	return pm.cgroupPidsMapping[relativeCgroupPath]
}
