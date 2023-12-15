// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package system

import (
	"bufio"
	"os"
	"regexp"

	"github.com/DataDog/datadog-agent/pkg/util/cgroups"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	selfMountInfoPath       = "/proc/self/mountinfo"
	containerdSandboxPrefix = "sandboxes"
	cIDRegexp               = `([^\s/]+)/(` + cgroups.ContainerRegexpStr + `)/[\S]*hostname`
)

var cIDMountInfoRegexp = regexp.MustCompile(cIDRegexp)

func getSelfContainerID(hostCgroupNamespace bool, cgroupVersion int, cgroupBaseController string) (string, error) {
	if cgroupVersion == 1 || hostCgroupNamespace {
		return cgroups.IdentiferFromCgroupReferences("/proc", cgroups.SelfCgroupIdentifier, cgroupBaseController, cgroups.ContainerFilter)
	}

	return parseMountinfo(selfMountInfoPath)
}

// Parsing /proc/self/mountinfo is not always reliable in Kubernetes+containerd (at least)
// We're still trying to use it as it may help in some cgroupv2 configurations (Docker, ECS, raw containerd)
func parseMountinfo(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := s.Text()
		allMatches := cIDMountInfoRegexp.FindAllStringSubmatch(line, -1)
		if len(allMatches) == 0 {
			continue
		}

		// We're interest in rightmost match
		matches := allMatches[len(allMatches)-1]
		if len(matches) > 0 && matches[1] != containerdSandboxPrefix {
			log.Infof("Found self container id: %s", matches[2])
			return matches[2], nil
		}
	}

	return "", nil
}
