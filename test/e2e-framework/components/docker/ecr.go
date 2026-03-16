// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package docker

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/namer"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	remoteComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// InstallECRCredentialsHelper installs the Amazon ECR credential helper on the given host
// and configures Docker to use it. This enables automatic authentication against ECR registries
// (including pull-through cache registries).
func InstallECRCredentialsHelper(n namer.Namer, host *remoteComp.Host, opts ...pulumi.ResourceOption) (command.Command, error) {
	ecrCredsHelperInstall, err := host.OS.PackageManager().Ensure("amazon-ecr-credential-helper", nil, "docker-credential-ecr-login", os.WithPulumiResourceOptions(opts...))
	if err != nil {
		return nil, err
	}

	ecrConfigCommand, err := host.OS.Runner().Command(
		n.ResourceName("ecr-config"),
		&command.Args{
			Create: pulumi.String("mkdir -p ~/.docker && echo '{\"credsStore\": \"ecr-login\"}' > ~/.docker/config.json"),
			Sudo:   false,
		},
		utils.MergeOptions(opts, utils.PulumiDependsOn(ecrCredsHelperInstall))...,
	)
	if err != nil {
		return nil, err
	}

	return ecrConfigCommand, nil
}
