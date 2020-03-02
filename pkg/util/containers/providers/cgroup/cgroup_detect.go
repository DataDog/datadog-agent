// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package cgroup

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	// cloudfoundry garden container have IDs in the form aaaaaaaa-bbbb-cccc-dddd-eeee
	containerRe = regexp.MustCompile("[0-9a-f]{64}|[0-9a-f]{8}(-[0-9a-f]{4}){4}")
	// ErrMissingTarget is an error set when a cgroup target is missing.
	ErrMissingTarget = errors.New("Missing cgroup target")
	// dindCgroupRe represents the cgroup pattern that the container runs inside a dind container,
	// the second capturing group is the correct path we need for cgroup path
	dindCgroupRe = regexp.MustCompile("^\\/docker\\/[0-9a-f]{64}(\\/docker\\/[0-9a-f]{64})")
)

// ContainerStartTime gets the stat for cgroup directory and use the mtime for that dir to determine the start time for the container
// this should work because the cgroup dir for the container would be created only when it's started
func (c ContainerCgroup) ContainerStartTime() (int64, error) {
	cgroupDir := c.cgroupFilePath("cpuacct", "")
	if !pathExists(cgroupDir) {
		return 0, fmt.Errorf("could not get cgroup dir, directory doesn't exist")
	}
	stat, err := os.Stat(cgroupDir)
	if err != nil {
		return 0, fmt.Errorf("could not get stat of the cgroup dir: %s", err)
	}
	return stat.ModTime().Unix(), nil
}

// cgroupFilePath constructs file path to get targeted stats file.
func (c ContainerCgroup) cgroupFilePath(target, file string) string {
	mount, ok := c.Mounts[target]
	if !ok {
		log.Debugf("Missing target %s from mounts", target)
		return ""
	}
	targetPath, ok := c.Paths[target]
	if !ok {
		log.Errorf("Missing target %s from paths", target)
		return ""
	}
	// sometimes the container is running inside a "dind container" instead of directly on the host,
	// we need to cover that case if the default full path doesn't exist
	// the dind container cgroup format looks like:
	//
	//	"/docker/$dind_container_id/docker/$container_id"
	//
	// and the actual cgroup path for that case is "docker/$container_id"
	if !pathExists(filepath.Join(mount, targetPath, file)) {
		if dindCgroupRe.MatchString(targetPath) {
			targetPath = dindCgroupRe.FindStringSubmatch(targetPath)[1]
		}
	}
	return filepath.Join(mount, targetPath, file)
}

// function to get the mount point of all cgroup. by default it should be under /sys/fs/cgroup but
// it could be mounted anywhere else if manually defined. Example cgroup entries in /proc/mounts would be
//	 cgroup /sys/fs/cgroup/cpuset cgroup rw,relatime,cpuset 0 0
//	 cgroup /sys/fs/cgroup/cpu cgroup rw,relatime,cpu 0 0
//	 cgroup /sys/fs/cgroup/cpuacct cgroup rw,relatime,cpuacct 0 0
//	 cgroup /sys/fs/cgroup/memory cgroup rw,relatime,memory 0 0
//	 cgroup /sys/fs/cgroup/devices cgroup rw,relatime,devices 0 0
//	 cgroup /sys/fs/cgroup/freezer cgroup rw,relatime,freezer 0 0
//	 cgroup /sys/fs/cgroup/blkio cgroup rw,relatime,blkio 0 0
//	 cgroup /sys/fs/cgroup/perf_event cgroup rw,relatime,perf_event 0 0
//	 cgroup /sys/fs/cgroup/hugetlb cgroup rw,relatime,hugetlb 0 0
//
// Returns a map for every target (cpuset, cpu, cpuacct) => path
func cgroupMountPoints() (map[string]string, error) {
	mountsFile := "/proc/mounts"
	if !pathExists(mountsFile) {
		return nil, fmt.Errorf("/proc/mounts does not exist")
	}
	f, err := os.Open(mountsFile)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return parseCgroupMountPoints(f), nil
}

func parseCgroupMountPoints(r io.Reader) map[string]string {
	cgroupRoot := config.Datadog.GetString("container_cgroup_root")
	mountPoints := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		mount := scanner.Text()
		tokens := strings.Split(mount, " ")
		// Check if the filesystem type is 'cgroup'
		if len(tokens) >= 3 && tokens[2] == "cgroup" {
			cgroupPath := tokens[1]

			// Ignore mountpoints not mounted under /{host/}sys
			if !strings.HasPrefix(cgroupPath, cgroupRoot) {
				continue
			}

			// Target can be comma-separate values like cpu,cpuacct
			tsp := strings.Split(path.Base(cgroupPath), ",")
			for _, target := range tsp {
				mountPoints[target] = cgroupPath
			}
		}
	}
	if len(mountPoints) == 0 {
		log.Warnf("No mountPoints were detected, current cgroup root is: %s", cgroupRoot)
	}
	return mountPoints
}

// ScrapeAllCgroups returns ContainerCgroup for every container that's in a Cgroup.
// This version iterates on /{host/}proc to retrieve processes out of the namespace.
// We return as a map[containerID]Cgroup for easy look-up.
func scrapeAllCgroups() (map[string]*ContainerCgroup, error) {
	mountPoints, err := cgroupMountPoints()
	if err != nil {
		return nil, err
	}

	cgs := make(map[string]*ContainerCgroup)

	// Opening proc dir
	procDir, err := os.Open(hostProc())
	if err != nil {
		return cgs, err
	}
	defer procDir.Close()
	dirNames, err := procDir.Readdirnames(-1)
	if err != nil {
		return cgs, err
	}

	prefix := config.Datadog.GetString("container_cgroup_prefix")

	for _, dirName := range dirNames {
		pid, err := strconv.ParseInt(dirName, 10, 32)
		if err != nil {
			continue
		}
		cgPath := hostProc(dirName, "cgroup")
		containerID, paths, err := readCgroupsForPath(cgPath, prefix)
		if containerID == "" {
			continue
		}
		if err != nil {
			log.Debugf("error reading cgroup paths %s: %s", cgPath, err)
			continue
		}
		if cg, ok := cgs[containerID]; ok {
			// Assumes that the paths will always be the same for a container id.
			cg.Pids = append(cg.Pids, int32(pid))
		} else {
			cgs[containerID] = &ContainerCgroup{
				ContainerID: containerID,
				Pids:        []int32{int32(pid)},
				Paths:       paths,
				Mounts:      mountPoints}
		}
	}
	return cgs, nil
}

// readCgroupsForPath reads the cgroups from a /proc/$pid/cgroup path.
func readCgroupsForPath(pidCgroupPath, prefix string) (string, map[string]string, error) {
	f, err := os.Open(pidCgroupPath)
	if os.IsNotExist(err) {
		log.Debugf("cgroup path '%s' could not be read: %s", pidCgroupPath, err)
		return "", nil, nil
	} else if err != nil {
		log.Debugf("cgroup path '%s' could not be read: %s", pidCgroupPath, err)
		return "", nil, err
	}
	defer f.Close()
	return parseCgroupPaths(f, prefix)
}

// parseCgroupPaths parses out the cgroup paths from a /proc/$pid/cgroup file.
// The file format will be something like:
//
// 11:net_cls:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e
// 10:freezer:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e
// 9:cpu,cpuacct:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e
// 8:memory:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e
// 7:blkio:/kubepods/besteffort/pod2baa3444-4d37-11e7-bd2f-080027d2bf10/47fc31db38b4fa0f4db44b99d0cad10e3cd4d5f142135a7721c1c95c1aadfb2e
//
// Returns the common containerID and a mapping of target => path
// If the first line doesn't have a valid container ID we will return an empty string
func parseCgroupPaths(r io.Reader, prefix string) (string, map[string]string, error) {
	var containerID string
	paths := make(map[string]string)
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		l := scanner.Text()
		cID, ok := containerIDFromCgroup(l, prefix)
		if !ok {
			log.Tracef("could not parse container id from path '%s'", l)
			continue
		}
		if containerID == "" {
			// Take the first valid containerID
			containerID = cID
		}
		sp := strings.SplitN(l, ":", 3)
		if len(sp) < 3 {
			continue
		}
		// Target can be comma-separate values like cpu,cpuacct
		tsp := strings.Split(sp[1], ",")
		for _, target := range tsp {
			paths[target] = sp[2]
		}
	}
	if err := scanner.Err(); err != nil {
		return "", nil, err
	}

	// In Ubuntu Xenial, we've encountered containers with no `cpu`
	_, cpuok := paths["cpu"]
	cpuacct, cpuacctok := paths["cpuacct"]
	if !cpuok && cpuacctok {
		paths["cpu"] = cpuacct
	}

	return containerID, paths, nil
}

func containerIDFromCgroup(cgroup, prefix string) (string, bool) {
	sp := strings.SplitN(cgroup, ":", 3)
	if len(sp) < 3 {
		return "", false
	}
	if prefix != "" && !strings.HasPrefix(sp[2], prefix) {
		return "", false
	}
	matches := containerRe.FindAllString(sp[2], -1)
	if matches == nil {
		return "", false
	}
	return matches[len(matches)-1], true
}
