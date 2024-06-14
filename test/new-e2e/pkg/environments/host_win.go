// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package environments

import (
	"github.com/DataDog/test-infra-definitions/resources/aws"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
)

// WindowsHost is an environment based on environments.Host but that is specific to Windows.
type WindowsHost struct {
	AwsEnvironment *aws.Environment
	// Components
	RemoteHost      *components.RemoteHost
	FakeIntake      *components.FakeIntake
	Agent           *components.RemoteHostAgent
	ActiveDirectory *components.RemoteActiveDirectory
}

var _ e2e.Initializable = &WindowsHost{}

// Init initializes the environment
func (e *WindowsHost) Init(_ e2e.Context) error {
	return nil
}
