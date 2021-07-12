// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build docker

package dockerswarm

import (
	"github.com/StackVista/stackstate-agent/pkg/util/containers"
)

// SwarmClient represents a docker client that can retrieve docker swarm information from the docker API
type SwarmClient interface {
	ListSwarmServices() ([]*containers.SwarmService, error)
}
