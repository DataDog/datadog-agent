// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package containerutils holds multiple utils functions around Container IDs and their patterns
package containerutils

import (
	"regexp"
	"slices"
	"strings"
)

// ContainerIDPatternStr defines the regexp used to match container IDs
// ([0-9a-fA-F]{64}) is standard container id used pretty much everywhere, length: 64
// ([0-9a-fA-F]{32}-\d+) is container id used by AWS ECS, length: 43
// ([0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){4}) is container id used by Garden, length: 28
var ContainerIDPatternStr = ""
var containerIDPattern *regexp.Regexp
var podUIDPattern *regexp.Regexp

var containerIDCoreChars = "0123456789abcdefABCDEF"
var podUIDBoundaryChars = "/-."

func init() {
	// when changing this pattern, make sure to also update the pre-check
	// in pkg/security/security_profile/activity_tree/paths_reducer.go

	ContainerIDPatternStr = "([0-9a-fA-F]{64})|([0-9a-fA-F]{32}-\\d+)|([0-9a-fA-F]{8}(-[0-9a-fA-F]{4}){4})"
	containerIDPattern = regexp.MustCompile(ContainerIDPatternStr)
	podUIDPattern = regexp.MustCompile(`pod([0-9a-fA-F]{8}[-_][0-9a-fA-F]{4}[-_][0-9a-fA-F]{4}[-_][0-9a-fA-F]{4}[-_][0-9a-fA-F]{12})`)
}

// FindContainerID extracts the first sub string that matches the pattern of a container ID along with the container flags induced from the container runtime prefix
func FindContainerID(s CGroupID) ContainerID {
	matches := containerIDPattern.FindAllIndex([]byte(s), -1)

	for _, match := range slices.Backward(matches) {
		// first, check what's before
		if match[0] != 0 {
			previousChar := string(s[match[0]-1])
			if strings.ContainsAny(previousChar, containerIDCoreChars) {
				continue
			}
		}
		// then, check what's after
		if match[1] < len(s) {
			nextChar := string(s[match[1]])
			if strings.ContainsAny(nextChar, containerIDCoreChars) {
				continue
			}
		}

		return ContainerID(s[match[0]:match[1]])
	}

	return ""
}

// FindPodUID extracts the Kubernetes pod UID from a kubelet cgroup path.
//
// It supports both cgroupfs paths such as:
//
//	/kubepods/besteffort/pod48d25824-cbe2-4fdc-9928-5bb49e05473d/...
//
// and systemd paths such as:
//
//	/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod48d25824_cbe2_4fdc_9928_5bb49e05473d.slice/...
//
// The returned UID is normalized to the canonical dashed, lowercase form.
func FindPodUID(s CGroupID) string {
	matches := podUIDPattern.FindAllStringSubmatchIndex(string(s), -1)

	for _, match := range slices.Backward(matches) {
		if len(match) < 4 {
			continue
		}

		podStart := match[0]
		uidStart := match[2]
		uidEnd := match[3]

		if podStart != 0 {
			previousChar := string(s[podStart-1])
			if !strings.ContainsAny(previousChar, podUIDBoundaryChars) {
				continue
			}
		}

		if uidEnd < len(s) {
			nextChar := string(s[uidEnd])
			if !strings.ContainsAny(nextChar, podUIDBoundaryChars) {
				continue
			}
		}

		return strings.ToLower(strings.ReplaceAll(string(s[uidStart:uidEnd]), "_", "-"))
	}

	return ""
}
