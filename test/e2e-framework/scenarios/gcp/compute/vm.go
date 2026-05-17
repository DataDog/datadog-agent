// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package compute

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp/compute"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp"
)

func NewVM(e gcp.Environment, name string, option ...VMOption) (*remote.Host, error) {
	params, paramsErr := newParams(option...)
	if paramsErr != nil {
		return nil, paramsErr
	}

	if err := defaultVMArgs(e, params); err != nil {
		return nil, err
	}

	imageName, err := resolveOS(e, params)
	if err != nil {
		return nil, err
	}

	readyFunc := command.WaitForSuccessfulConnection
	startupScript := ""
	switch params.osInfo.Flavor {
	case os.Ubuntu, os.Debian:
		// Wait for cloud-init to finish on Debian-based images before running any
		// package manager commands. Otherwise early apt operations can race with
		// boot-time updates on fresh GCE VMs and fail intermittently.
		readyFunc = command.WaitForCloudInit
		startupScript = os.APTDisableUnattendedUpgradesScriptContent
	case os.Suse:
		startupScript = os.ZypperDisableUnattendedUpgradesScriptContent
	}

	return components.NewComponent(&e, name, func(h *remote.Host) error {
		h.CloudProvider = pulumi.String(components.CloudProviderGCP).ToStringOutput()
		vm, err := compute.NewLinuxInstance(
			e,
			e.Namer.ResourceName(name),
			imageName,
			startupScript,
			params.instanceType,
			params.nestedVirt,
			params.labels,
			pulumi.Parent(h),
		)
		if err != nil {
			return err
		}

		conn, err := remote.NewConnection(
			vm.NetworkInterfaces.Index(pulumi.Int(0)).NetworkIp().Elem(),
			"gce",
			remote.WithPrivateKeyPath(e.DefaultPrivateKeyPath()),
			remote.WithPrivateKeyPassword(e.DefaultPrivateKeyPassword()),
			remote.WithDialErrorLimit(e.InfraDialErrorLimit()),
			remote.WithPerDialTimeoutSeconds(e.InfraPerDialTimeoutSeconds()),
		)
		if err != nil {
			return err
		}

		return remote.InitHost(&e, conn.ToConnectionOutput(), *params.osInfo, "gce", pulumi.String("").ToStringOutput(), readyFunc, h)
	})
}

func defaultVMArgs(e gcp.Environment, vmArgs *vmArgs) error {
	if vmArgs.osInfo == nil {
		vmArgs.osInfo = &os.UbuntuDefault
	}

	if vmArgs.instanceType == "" {
		vmArgs.instanceType = e.DefaultInstanceType()
	}

	return nil
}

func resolveOS(e gcp.Environment, vmArgs *vmArgs) (string, error) {
	if vmArgs.imageName == "" {
		resolver, ok := imageResolvers[vmArgs.osInfo.Flavor]
		if !ok {
			return "", fmt.Errorf("unsupported OS flavor %v", vmArgs.osInfo.Flavor)
		}
		image, err := resolver(e, *vmArgs.osInfo)
		if err != nil {
			return "", err
		}
		return image, nil
	}
	return vmArgs.imageName, nil
}
