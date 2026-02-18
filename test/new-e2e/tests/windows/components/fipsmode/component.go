// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fipsmode contains code to control the behavior of Windows FIPS mode in the E2E tests
package fipsmode

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/powershell"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Manager contains the resources to manage Windows FIPS mode
//
// https://learn.microsoft.com/en-us/previous-versions/windows/it-pro/windows-10/security/threat-protection/security-policy-settings/system-cryptography-use-fips-compliant-algorithms-for-encryption-hashing-and-signing
type Manager struct {
	namer namer.Namer
	host  *remote.Host

	Resources []pulumi.Resource
}

// New creates a new instance of the Windows FIPS mode component
func New(e *config.CommonEnvironment, host *remote.Host, options ...Option) (*Manager, error) {
	params, err := common.ApplyOption(&Configuration{}, options)
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		namer: e.CommonNamer().WithPrefix("windows-fips-mode"),
		host:  host,
	}

	if params.FIPSModeEnabled {
		cmd, err := host.OS.Runner().Command(manager.namer.ResourceName("enable"), &command.Args{
			Create: pulumi.String(powershell.PsHost().
				SetFIPSMode(true).
				Compile()),
			Delete: pulumi.String(powershell.PsHost().
				SetFIPSMode(false).
				Compile()),
		})
		if err != nil {
			return nil, err
		}
		manager.Resources = append(manager.Resources, cmd)
	}

	return manager, nil
}
