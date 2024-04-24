// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secretsutils contains utilities to manage secrets for e2e tests.
package secretsutils

import (
	_ "embed"

	"github.com/DataDog/datadog-agent/pkg/util/optional"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	perms "github.com/DataDog/test-infra-definitions/components/datadog/agentparams/filepermissions"
)

//go:embed fixtures/secret-resolver.py
var secretResolverScript []byte

// WithUnixSecretSetupScript returns an agent param that setups a secret resolver script with correct permissions.
func WithUnixSecretSetupScript(path string, allowGroupExec bool) func(*agentparams.Params) error {
	return agentparams.WithFileWithPermissions(path, string(secretResolverScript), true, WithUnixSecretPermissions(allowGroupExec))
}

// WithUnixSecretPermissions returns an UnixPermissions object containing correct permissions for a secret backend script.
func WithUnixSecretPermissions(allowGroupExec bool) optional.Option[perms.FilePermissions] {
	if allowGroupExec {
		return perms.NewUnixPermissions(perms.WithPermissions("0750"), perms.WithOwner("dd-agent"), perms.WithGroup("root"))
	}

	return perms.NewUnixPermissions(perms.WithPermissions("0700"), perms.WithOwner("dd-agent"), perms.WithGroup("dd-agent"))
}

// WithWindowsSecretPermissions returns a WindowsPermissions object containing correct permissions for a secret backend script.
func WithWindowsSecretPermissions(allowGroupExec bool) optional.Option[perms.FilePermissions] {
	icaclsCmd := `/grant "ddagentuser:(RX)"`
	if allowGroupExec {
		icaclsCmd += ` "Administrators:(RX)"`
	}

	return perms.NewWindowsPermissions(perms.WithIcaclsCommand(icaclsCmd), perms.WithDisableInheritance())
}
