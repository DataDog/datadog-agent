// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package docker

import (
	"maps"
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const rancherIPLabel = "io.rancher.container.ip"

// FindRancherIPInLabels looks for the `io.rancher.container.ip` label and parses it.
// Rancher 1.x containers don't have docker networks as the orchestrator provides its own CNI.
func FindRancherIPInLabels(labels map[string]string) (string, bool) {
	cidr, found := labels[rancherIPLabel]
	if found {
		ipv4Addr, _, err := net.ParseCIDR(cidr)
		if err != nil {
			log.Warnf("error while retrieving Rancher IP: %q is not valid", cidr)
			return "", false
		}
		return ipv4Addr.String(), true
	}

	return "", false
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
