// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package activedirectory contains the code necessary to create an Active Directory environment for e2e tests
package activedirectory

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/test-infra-definitions/common/config"
)

// Env represents an Active Directory environment for an e2e test
type Env struct {
	DomainControllerHost *components.RemoteHost
	DomainController     *RemoteActiveDirectory
	FakeIntake           *components.FakeIntake
	Environment          *config.CommonEnvironment
}

// RemoteActiveDirectory represents an Active Directory domain setup on a remote host
type RemoteActiveDirectory struct {
	Output
}
