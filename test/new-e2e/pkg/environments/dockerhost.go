// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/common"
)

// DockerHost is an environment that contains a Docker VM, FakeIntake and Agent configured to talk to each other.
type DockerHost struct {
	// Components
	RemoteHost *components.RemoteHost
	FakeIntake *components.FakeIntake
	Agent      *components.DockerAgent
	Docker     *components.RemoteHostDocker
}

var _ common.Initializable = &DockerHost{}

// Init initializes the environment
func (e *DockerHost) Init(_ common.Context) error {
	return nil
}
