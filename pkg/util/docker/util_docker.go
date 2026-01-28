// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker

package docker

import (
	"errors"
	"fmt"
	"maps"

	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/volume"
)

// buildDockerFilter creates a filter.Args object from an even
// number of strings, used as key, value pairs
// An empty "catch-all" filter can be created by passing no argument
func buildDockerFilter(args ...string) (volume.ListOptions, error) {
	filter := filters.NewArgs()
	if len(args)%2 != 0 {
		return volume.ListOptions{Filters: filter}, errors.New("an even number of arguments is required")
	}
	for i := 0; i < len(args); i += 2 {
		filter.Add(args[i], args[i+1])
	}
	return volume.ListOptions{Filters: filter}, nil
}

// GetInspectCacheKey returns the key to a given container ID inspect in the agent cache
func GetInspectCacheKey(ID string, withSize bool) string {
	return fmt.Sprintf("dockerutil.containers.%s.withsize.%t", ID, withSize)
}

// ContainerHosts returns a map of hostnames to IP addresses for a container.
// It includes the container's network IP addresses, the rancher IP if
// available, and container's hostname if no IP is available.
func ContainerHosts(networkIPs, labels map[string]string, hostname string) map[string]string {
	hosts := make(map[string]string)

	maps.Copy(hosts, networkIPs)

	if rancherIP, ok := FindRancherIPInLabels(labels); ok {
		hosts["rancher"] = rancherIP
	}

	// Some CNI solutions (including ECS awsvpc) do not assign an
	// IP through docker, but set a valid reachable hostname. Use
	// it if no IP is discovered.
	if len(hosts) == 0 && len(hostname) > 0 {
		hosts["hostname"] = hostname
	}
	return hosts
}
