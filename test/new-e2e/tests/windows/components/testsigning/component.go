// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package testsigning contains code to control the behavior of Windows test signing in the E2E tests
package testsigning

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumiverse/pulumi-time/sdk/go/time"

	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common/powershell"
)

// Manager contains the resources to manage Windows TestSigning
type Manager struct {
	namer namer.Namer
	host  *remote.Host

	Resources []pulumi.Resource
}

// NewTestSigning creates a new instance of the Windows TestSigning component
func NewTestSigning(e *config.CommonEnvironment, host *remote.Host, options ...Option) (*Manager, error) {
	params, err := common.ApplyOption(&Configuration{}, options)
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		namer: e.CommonNamer().WithPrefix("windows-test-signing"),
		host:  host,
	}

	if params.Enabled {
		cmd, err := host.OS.Runner().Command(manager.namer.ResourceName("enable-test-signing"), &command.Args{
			Create: pulumi.String(powershell.PsHost().
				EnableTestSigning().
				Compile()),
		}, params.ResourceOptions...)
		if err != nil {
			return nil, err
		}
		manager.Resources = append(manager.Resources, cmd)

		timeProvider, err := time.NewProvider(e.Ctx(), manager.namer.ResourceName("time-provider"), &time.ProviderArgs{}, pulumi.DeletedWith(host))
		if err != nil {
			return nil, err
		}
		params.ResourceOptions = append(params.ResourceOptions, pulumi.DependsOn(manager.Resources), pulumi.Provider(timeProvider))

		waitForRebootCmd, err := time.NewSleep(e.Ctx(), manager.namer.ResourceName("wait-for-host-to-reboot"), &time.SleepArgs{
			CreateDuration: pulumi.String("60s"),
		}, params.ResourceOptions...)
		if err != nil {
			return nil, err
		}
		manager.Resources = append(manager.Resources, waitForRebootCmd)
	}

	return manager, nil
}
