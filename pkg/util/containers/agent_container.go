// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package containers

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"regexp"
)

// cgroupPath is the path from which we parse the container id
const cgroupPath string = "/proc/self/cgroup"

var (
	// expLine matches a line in the /proc/self/cgroup file. It has a submatch for the last element (path), which contains the container ID.
	expLine = regexp.MustCompile(`^\d+:[^:]*:(.+)$`)
	// expContainerID matches contained IDs and sources. Source: https://github.com/Qard/container-info/blob/master/index.js
	expContainerID = regexp.MustCompile(`([0-9a-f]{8}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{4}[-_][0-9a-f]{12}|[0-9a-f]{64})(?:.scope)?$`)
)

// GetAgentContainerID tries to discover the agent container id by parsing /proc/self/cgroup
func GetAgentContainerID() (string, error) {
	return parseContainerID(cgroupPath)
}

// parseContainerID parses a given path to get the container id
// separated from GetAgentContainerID for testing purpose
func parseContainerID(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot open %s: %v", path, err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		path := expLine.FindStringSubmatch(scanner.Text())
		if len(path) != 2 {
			// invalid entry, continue
			continue
		}
		if id := expContainerID.FindString(path[1]); id != "" {
			return id, nil
		}
	}
	return "", errors.New("no valid container id is found")
}
