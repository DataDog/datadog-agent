// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containerutils holds multiple utils functions around Container IDs and their patterns
package containerutils

import (
	"path/filepath"
	"regexp"
	"strings"
)

// CGroup managers
const (
	CGroupManagerDocker  uint64 = 1 << 0
	CGroupManagerCRIO    uint64 = 2 << 1
	CGroupManagerPodman  uint64 = 3 << 2
	CGroupManagerCRI     uint64 = 4 << 3
	CGroupManagerSystemd uint64 = 5 << 4
)

var runtimePrefixes = map[string]uint64{
	"docker-":         CGroupManagerDocker,
	"cri-containerd-": CGroupManagerCRI,
	"crio-":           CGroupManagerCRIO,
	"libpod-":         CGroupManagerPodman,
}

// ContainerIDPatternStr defines the regexp used to match container IDs
// ([0-9a-fA-F]{64}) is standard container id used pretty much everywhere, length: 64
// ([0-9a-fA-F]{32}-\d+) is container id used by AWS ECS, length: 43
// ([0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){4}) is container id used by Garden, length: 28
var ContainerIDPatternStr string
var containerIDPattern *regexp.Regexp

var containerIDCoreChars = "0123456789abcdefABCDEF"

func init() {
	var prefixes []string
	for prefix := range runtimePrefixes {
		prefixes = append(prefixes, prefix)
	}
	ContainerIDPatternStr = "(" + strings.Join(prefixes[:], "|") + ")?([0-9a-fA-F]{64})|([0-9a-fA-F]{32}-\\d+)|([0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){4})"
	containerIDPattern = regexp.MustCompile(ContainerIDPatternStr)
}

// FindContainerID extracts the first sub string that matches the pattern of a container ID
func FindContainerID(s string) (string, uint64) {
	match := containerIDPattern.FindIndex([]byte(s))
	if match == nil {
		// Is it a systemd cgroup ?
		if strings.HasSuffix(s, ".service") {
			service := strings.TrimSuffix(filepath.Base(s), ".service")
			return service, CGroupManagerSystemd
		}
		if strings.HasSuffix(s, ".scope") {
			// ignore for now
			return "", CGroupManagerSystemd
		}
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
	var flags uint64
	containerID := s[match[0]:match[1]]
	for runtimePrefix, runtimeFlag := range runtimePrefixes {
		if strings.HasPrefix(containerID, runtimePrefix) {
			flags = runtimeFlag
			containerID = containerID[len(runtimePrefix):]
			break
		}
	}

	return containerID, flags
}
