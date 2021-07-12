// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package providers

import (
	"github.com/StackVista/stackstate-agent/pkg/util/containers"
	"github.com/StackVista/stackstate-agent/pkg/util/log"
)

// ContainerImpl without implementation
// Implementations should could Register() in their init()
var containerImpl containers.ContainerImplementation

// ContainerImpl returns the ContainerImplementation
func ContainerImpl() containers.ContainerImplementation {
	if containerImpl == nil {
		panic("Trying to get nil ContainerInterface")
	}

	return containerImpl
}

// Register allows to set a ContainerImplementation
func Register(impl containers.ContainerImplementation) {
	if containerImpl == nil {
		containerImpl = impl
	} else {
		log.Critical("Trying to set multiple ContainerImplementation")
	}
}
