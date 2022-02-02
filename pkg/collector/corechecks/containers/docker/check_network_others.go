// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build docker && !linux
// +build docker,!linux

package docker

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/containers/generic"
	dockerTypes "github.com/docker/docker/api/types"
)

// Nothing to do here. Only the linux implementation has specificites
func (d *DockerCheck) configureNetworkProcessor(processor *generic.Processor) {
}

type dockerNetworkExtension struct{}

func (dn dockerNetworkExtension) preRun() {
}

func (dn dockerNetworkExtension) processContainer(rawContainer dockerTypes.Container) {
}

func (dn dockerNetworkExtension) postRun() {
}
