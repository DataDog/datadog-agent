// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containerutils holds multiple utils functions around Container IDs and their patterns
package containerutils

import (
	"regexp"
	"strings"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
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
	for prefix := range model.RuntimePrefixes {
		prefixes = append(prefixes, prefix)
	}
	ContainerIDPatternStr = "(?:" + strings.Join(prefixes[:], "|") + ")?([0-9a-fA-F]{64})|([0-9a-fA-F]{32}-\\d+)|([0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){4})"
	containerIDPattern = regexp.MustCompile(ContainerIDPatternStr)
}

// FindContainerID extracts the first sub string that matches the pattern of a container ID
func FindContainerID(s string) (string, uint64) {
	match := containerIDPattern.FindIndex([]byte(s))
	if match == nil {
		return "", 0
	}

	// first, check what's before
	if match[0] != 0 {
		previousChar := string(s[match[0]-1])
		if strings.ContainsAny(previousChar, containerIDCoreChars) {
			return "", 0
		}
	}
	// then, check what's after
	if match[1] < len(s) {
		nextChar := string(s[match[1]])
		if strings.ContainsAny(nextChar, containerIDCoreChars) {
			return "", 0
		}
	}

	// ensure the found containerID is delimited by charaters other than a-zA-Z0-9, or that
	// it starts or/and ends the initial string

	cgroupID := s[match[0]:match[1]]
	containerID, flags := model.GetContainerFromCgroup(cgroupID)
	return containerID, flags
}

// GetContainerRuntime returns the container runtime managing the cgroup
func GetContainerRuntime(flags uint64) string {
	switch {
	case (flags & model.CGroupManagerCRI) != 0:
		return string(workloadmeta.ContainerRuntimeContainerd)
	case (flags & model.CGroupManagerCRIO) != 0:
		return string(workloadmeta.ContainerRuntimeCRIO)
	case (flags & model.CGroupManagerDocker) != 0:
		return string(workloadmeta.ContainerRuntimeDocker)
	case (flags & model.CGroupManagerPodman) != 0:
		return string(workloadmeta.ContainerRuntimePodman)
	default:
		return ""
	}
}

// GetCGroupContext returns the cgroup ID and the sanitized container ID from a container id/flags tuple
func GetCGroupContext(containerID string, containerFlags uint64) (string, string) {
	cgroupID := model.GetCgroupFromContainer(containerID, uint64(containerFlags))
	if containerFlags&0b111 == 0 {
		containerID = ""
	}
	return cgroupID, containerID
}
