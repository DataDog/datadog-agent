// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	procPIDMapperID = "proc"
)

// IdentiferFromCgroupReferences returns cgroup identifier extracted from <proc>/<pid>/cgroup after applying the filter.
func IdentiferFromCgroupReferences(procPath, pid, baseCgroupController string, filter ReaderFilter) (string, error) {
	var identifier string

	err := parseFile(defaultFileReader, filepath.Join(procPath, pid, procCgroupFile), func(s string) error {
		var err error

		parts := strings.SplitN(s, ":", 3)
		// Skip potentially malformed lines
		if len(parts) != 3 {
			return nil
		}

		if parts[1] != baseCgroupController {
			return nil
		}

		// We need to remove first / as the path produced in Readers may not include it
		relativeCgroupPath := strings.TrimLeft(parts[2], "/")
		identifier, err = filter(relativeCgroupPath, filepath.Base(relativeCgroupPath))
		if err != nil {
			return err
		}

		return &stopParsingError{}
	})
	if err != nil {
		return "", err
	}
	return identifier, err
}

// Unfortunately, the reading of `<host_path>/sys/fs/cgroup/pids/.../cgroup.procs` is PID-namespace aware,
// meaning that we cannot rely on it to find all PIDs belonging to a cgroup, except if the Agent runs in host PID namespace.
type pidMapper interface {
	getPIDsForCgroup(identifier, relativeCgroupPath string, cacheValidity time.Duration) []int
}

// cgroupRoot is cgroup base directory (like /host/sys/fs/cgroup/<baseController>)
func getPidMapper(procPath, cgroupRoot, baseController string, filter ReaderFilter, pidMapperID string) pidMapper {
	// Empty pidMapperID means auto select. Only possible value is to force /proc usage.
	if pidMapperID == "" {
		// Checking if we are in host pid. If that's the case `cgroup.procs` in any controller will contain PIDs
		// In cgroupv2, the file contains 0 values, filtering for that
		cgroupProcsTestFilePath := filepath.Join(cgroupRoot, cgroupProcsFile)
		cgroupProcsUsable := false
		err := parseFile(defaultFileReader, cgroupProcsTestFilePath, func(s string) error {
			if s != "" && s != "0" {
				cgroupProcsUsable = true
			}

			return nil
		})

		if cgroupProcsUsable {
			log.Debug("Using cgroup.procs for pid mapping")
			return &cgroupProcsPidMapper{
				fr: defaultFileReader,
				cgroupProcsFilePathBuilder: func(relativeCgroupPath string) string {
					return filepath.Join(cgroupRoot, relativeCgroupPath, cgroupProcsFile)
				},
			}
		}
		log.Debugf("cgroup.procs file at: %s is empty or unreadable, considering we're not running in host PID namespace, err: %v", cgroupProcsTestFilePath, err)
	} else if pidMapperID != procPIDMapperID {
		log.Warnf("Unknown PID mapper ID: %s, falling back to using proc PID mapper", pidMapperID)
	}

	// Checking if we're in host cgroup namespace, other the method below cannot be used either
	// (we'll still return it in case the cgroup namespace detection failed but log a warning)
	log.Debug("Using proc/pid for pid mapping")
	pidMapper := &procPidMapper{
		procPath:         procPath,
		cgroupController: baseController,
		readerFilter:     filter,
	}

	// In cgroupv2, checking if we run in host cgroup namespace.
	// If not we cannot fill PIDs for containers and do PID<>CID mapping.
	if baseController == "" {
		cgroupInode, err := getProcessNamespaceInode("/proc", "self", "cgroup")
		if err == nil {
			if isHostNs := IsProcessHostCgroupNamespace(procPath, cgroupInode); isHostNs != nil && !*isHostNs {
				log.Warnf("Usage of cgroupv2 detected but the Agent does not seem to run in host cgroup namespace. Make sure to run with --cgroupns=host, some feature may not work otherwise")
			}
		} else {
			log.Debugf("Unable to get self cgroup namespace inode, err: %v", err)
		}
	}

	return pidMapper
}

// Mapper used if we are running in host PID namespace and with access to cgroup FS, faster.
type cgroupProcsPidMapper struct {
	fr fileReader
	// args are: relative cgroup path
	cgroupProcsFilePathBuilder func(string) string
}

func (pm *cgroupProcsPidMapper) getPIDsForCgroup(_, relativeCgroupPath string, _ time.Duration) []int {
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

	// If no PIDs found and this is a Podman cgroup, check container subdirectory
	// Podman rootless places PIDs in a "container" subdirectory within libpod-* cgroups
	if len(pids) == 0 && strings.Contains(relativeCgroupPath, "libpod-") {
		containerPath := filepath.Join(filepath.Dir(pm.cgroupProcsFilePathBuilder(relativeCgroupPath)), "container", cgroupProcsFile)
		if err := parseFile(pm.fr, containerPath, func(s string) error {
			pid, err := strconv.Atoi(s)
			if err != nil {
				reportError(newValueError(s, err))
				return nil
			}

			pids = append(pids, pid)

			return nil
		}); err != nil {
			// Only log if it's not a "file not found" error (expected if container/ doesn't exist)
			if !errors.Is(err, os.ErrNotExist) {
				reportError(err)
			}
		}
	}

	return pids
}

// Mapper used if we are NOT running in host PID namespace (most common cases in containers)
type procPidMapper struct {
	lock              sync.Mutex
	refreshTimestamp  time.Time
	procPath          string
	cgroupController  string
	readerFilter      ReaderFilter
	cgroupPidsMapping map[string][]int
}

func (pm *procPidMapper) refreshMapping(cacheValidity time.Duration) {
	if pm.refreshTimestamp.Add(cacheValidity).After(time.Now()) {
		return
	}

	cgroupPidMapping := make(map[string][]int)

	// Going through everything in `<procPath>/<pid>/cgroup`
	err := filepath.WalkDir(pm.procPath, func(_ string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if de.Name() == "proc" {
			return nil
		}

		pid, err := strconv.ParseInt(de.Name(), 10, 0)
		if err != nil {
			return skipThis(de)
		}

		cgroupIdentifier, err := IdentiferFromCgroupReferences(pm.procPath, de.Name(), pm.cgroupController, pm.readerFilter)
		if err != nil {
			log.Debugf("Unable to parse cgroup file for pid: %s, err: %v", de.Name(), err)
		}
		if cgroupIdentifier != "" {
			cgroupPidMapping[cgroupIdentifier] = append(cgroupPidMapping[cgroupIdentifier], int(pid))
		}

		return skipThis(de)
	})

	pm.refreshTimestamp = time.Now()
	if err != nil {
		reportError(err)
	} else {
		pm.cgroupPidsMapping = cgroupPidMapping
	}
}

// skipThis is a helper to skip only the currently processed entry.
// filepath.SkipDir will skip the given entry if it's a directory,
// but it will skip unvisited entries of the parent if it's a file
// which is not what we want usually.
func skipThis(de fs.DirEntry) error {
	if de.IsDir() {
		return filepath.SkipDir
	}
	return nil
}

//nolint:revive // TODO(CINT) Fix revive linter
func (pm *procPidMapper) getPIDsForCgroup(identifier, relativeCgroupPath string, cacheValidity time.Duration) []int {
	pm.lock.Lock()
	defer pm.lock.Unlock()

	pm.refreshMapping(cacheValidity)
	return pm.cgroupPidsMapping[identifier]
}

// StandalonePIDMapper allows to get a PID Mapper that could work without cgroup objects.
// This is required when PID namespace is shared but cgroup namespace is not (typically in serverless-like scenarios)
type StandalonePIDMapper interface {
	GetPIDs(cgroupIdentifier string, cacheValidity time.Duration) []int
}

type standalonePIDMapper struct {
	procPidMapper
}

// NewStandalonePIDMapper returns a new instance
func NewStandalonePIDMapper(procPath, cgroupController string, filter ReaderFilter) StandalonePIDMapper {
	return &standalonePIDMapper{
		procPidMapper: procPidMapper{
			procPath:         procPath,
			cgroupController: cgroupController,
			readerFilter:     filter,
		},
	}
}

// GetPIDs returns list of PID for a cgroup identifier, thread safe.
func (pm *standalonePIDMapper) GetPIDs(cgroupIdentifier string, cacheValidity time.Duration) []int {
	return pm.getPIDsForCgroup(cgroupIdentifier, "", cacheValidity)
}
