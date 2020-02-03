// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package providers

import (
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/containers/providers/cgroup"
)

// ContainerImpl provides a ContainerImplementation for Linux builds
var ContainerImpl containers.ContainerImplementation

func init() {
	ContainerImpl = &cgroup.Provider{}
}
