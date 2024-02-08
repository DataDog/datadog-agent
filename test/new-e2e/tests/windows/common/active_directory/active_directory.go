// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package active_directory

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/test-infra-definitions/common/config"
)

type ActiveDirectoryEnv struct {
	DomainControllerHost *components.RemoteHost
	DomainController     *RemoteActiveDirectory
	FakeIntake           *components.FakeIntake
	Environment          *config.CommonEnvironment
}

// RemoteActiveDirectory represents an Active Directory domain setup on a remote host
type RemoteActiveDirectory struct {
	ActiveDirectoryOutput
}
