// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd

package containerd

import (
	"errors"
	"net"

	containerd "github.com/containerd/containerd/v2/client"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	cutil "github.com/DataDog/datadog-agent/pkg/util/containerd"
	"github.com/DataDog/datadog-agent/pkg/util/system"
)

// extractIP gets the IP of a container.
//
// The containerd client does not expose the IPs, that's why we use the helpers
// that we have in the "system" package to extract that information from
// "/proc".
//
// Limitations:
//   - Host /proc needs to be mounted.
//   - If the container exposes multiple IPs, this function just returns one of
//     them, preferring non link-local ones. That means that if a container is
//     attached to multiple networks this might not work as expected.
func extractIP(namespace string, container containerd.Container, containerdClient cutil.ContainerdItf) (string, error) {
	if !env.IsHostProcAvailable() {
		return "", nil
	}

	taskPids, err := containerdClient.TaskPids(namespace, container)
	if err != nil {
		return "", err
	}

	if len(taskPids) == 0 {
		return "", errors.New("no PIDs found")
	}

	// Any PID should work, but processes could be deleted between we retrieve
	// the list of PIDs and the moment we check the files. That's why we try all
	// of them.
	for _, taskPid := range taskPids {
		IPs, err := system.ParseProcessIPs(
			pkgconfigsetup.Datadog().GetString("container_proc_root"),
			int(taskPid.Pid),
			func(ip string) bool { return ip != "127.0.0.1" },
		)
		if err != nil {
			continue
		}

		if len(IPs) == 0 {
			return "", errors.New("no IPs found")
		}

		return selectIP(IPs), nil
	}

	return "", errors.New("no IPs found")
}

// selectIP returns the first non link-local IP, or the first IP if they are
// all link-local. ECS awsvpc tasks have a 169.254.172.x address next to the
// task ENI IP, and only the latter is reachable from the agent.
func selectIP(IPs []string) string {
	for _, ip := range IPs {
		if parsed := net.ParseIP(ip); parsed != nil && !parsed.IsLinkLocalUnicast() {
			return ip
		}
	}
	return IPs[0]
}
