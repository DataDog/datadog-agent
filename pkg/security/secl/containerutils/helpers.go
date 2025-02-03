// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containerutils holds multiple utils functions around Container IDs and their patterns
package containerutils

import (
	"regexp"
	"strings"
)

// ContainerIDPatternStr defines the regexp used to match container IDs
// ([0-9a-fA-F]{64}) is standard container id used pretty much everywhere, length: 64
// ([0-9a-fA-F]{32}-\d+) is container id used by AWS ECS, length: 43
// ([0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){4}) is container id used by Garden, length: 28
var ContainerIDPatternStr = ""
var containerIDPattern *regexp.Regexp

var containerIDCoreChars = "0123456789abcdefABCDEF"

func init() {
	var prefixes []string
	for _, runtimePrefix := range RuntimePrefixes {
		prefixes = append(prefixes, runtimePrefix.prefix)
	}
	ContainerIDPatternStr = "(?:" + strings.Join(prefixes[:], "|") + ")?([0-9a-fA-F]{64})|([0-9a-fA-F]{32}-\\d+)|([0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){4})"
	containerIDPattern = regexp.MustCompile(ContainerIDPatternStr)
}

func isSystemdScope(cgroup CGroupID) bool {
	return strings.HasSuffix(string(cgroup), ".scope")
}

func isSystemdService(cgroup CGroupID) bool {
	return strings.HasSuffix(string(cgroup), ".service")
}

func getSystemdCGroupFlags(cgroup CGroupID) uint64 {
	if isSystemdScope(cgroup) {
		return uint64(CGroupManagerSystemd) | uint64(SystemdScope)
	} else if isSystemdService(cgroup) {
		return uint64(CGroupManagerSystemd) | uint64(SystemdService)
	}
	return 0
}

// FindContainerID extracts the first sub string that matches the pattern of a container ID along with the container flags induced from the container runtime prefix
func FindContainerID(s CGroupID) (ContainerID, uint64) {
	match := containerIDPattern.FindIndex([]byte(s))
	if match == nil {
		return "", getSystemdCGroupFlags(s)
	}

	// first, check what's before
	if match[0] != 0 {
		previousChar := string(s[match[0]-1])
		if strings.ContainsAny(previousChar, containerIDCoreChars) {
			return "", getSystemdCGroupFlags(s)
		}
	}
	// then, check what's after
	if match[1] < len(s) {
		nextChar := string(s[match[1]])
		if strings.ContainsAny(nextChar, containerIDCoreChars) {
			return "", getSystemdCGroupFlags(s)
		}
	}

	// ensure the found containerID is delimited by characters other than a-zA-Z0-9, or that
	// it starts or/and ends the initial string

	cgroupID := s[match[0]:match[1]]
	containerID, flags := getContainerFromCgroup(CGroupID(cgroupID))
	if containerID == "" {
		return ContainerID(cgroupID), uint64(flags)
	}

	return containerID, uint64(flags)
}
