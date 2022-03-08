// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package util

import "github.com/DataDog/datadog-agent/pkg/config"

// LogWhat is the answer to ContainersOrPods
type LogWhat int

const (
	// LogContainers means that the logs-agent should log containers, not pods.
	LogContainers LogWhat = iota

	// LogPods means that the logs-agent should log pods, not containers.
	LogPods

	// LogNothing means neither containers nor pods are supported.
	LogNothing
)

// ContainersOrPods determines how the logs-agent should handle
// containers: either monitoring individual containers, or monitoring pods and
// logging the containers within them.
func ContainersOrPods() LogWhat {
	c := config.IsFeaturePresent(config.Docker) ||
		config.IsFeaturePresent(config.Containerd) ||
		config.IsFeaturePresent(config.Cri) ||
		config.IsFeaturePresent(config.Podman)
	k := config.IsFeaturePresent(config.Kubernetes)

	switch {
	case c && !k:
		return LogContainers
	case k && !c:
		return LogPods
	case k && c:
		// prefer kubernetes if k8s_container_use_file is set
		if config.Datadog.GetBool("logs_config.k8s_container_use_file") {
			return LogPods
		}
		return LogContainers
	}

	return LogNothing
}
