// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// This file installs at runtime the dependencies that the standard
// Ubuntu2204E2E AMI ships pre-baked but that the GPU-specific NVIDIA-driver
// AMIs do not yet include. This includes Docker itself — the NVIDIA driver
// AMIs are bare Ubuntu images with CUDA; Docker is not pre-installed.
//
// TEMPORARY: this exists so the GPU e2e suite can keep running while we wait
// for ami-builder to ship GPU AMI variants (`ubuntu/22-04-gpu-e2e` and
// `ubuntu/18-04-gpu-e2e`) layering our e2e tooling on top of the GPU base
// image. Once those AMIs land and the systemData entries in this package
// point at them, delete this file and the two `installGPURuntimeDeps` call
// sites in provisioner.go. Tracked in ACIX-1305.

package gpu

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	componentsremote "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
)

// gpuRuntimeComposeVersion must match the version checked by docker.assertCompose.
const gpuRuntimeComposeVersion = "v2.27.0"

// installGPURuntimeDeps installs Docker, jq, amazon-ecr-credential-helper, and
// docker-compose at runtime. Wire its returned Command as a Pulumi dependency
// on any subsequent docker / ECR setup so the inner version checks pass.
//
// TEMPORARY — see file header.
func installGPURuntimeDeps(awsEnv *aws.Environment, host *componentsremote.Host) (command.Command, error) {
	dockerInstall, err := docker.InstallDocker(host)
	if err != nil {
		return nil, err
	}

	jqInstall, err := host.OS.PackageManager().Ensure("jq", nil, "jq")
	if err != nil {
		return nil, err
	}

	ecrHelperInstall, err := host.OS.PackageManager().Ensure("amazon-ecr-credential-helper", nil, "docker-credential-ecr-login")
	if err != nil {
		return nil, err
	}

	composeInstall, err := host.OS.Runner().Command(
		awsEnv.Namer.ResourceName("gpu-runtime-install-compose"),
		&command.Args{
			Create: pulumi.Sprintf(
				"bash -c '(docker-compose version | grep %s) || (curl --retry 10 -fsSLo /usr/local/bin/docker-compose https://github.com/docker/compose/releases/download/%s/docker-compose-linux-$(uname -p) && sudo chmod 755 /usr/local/bin/docker-compose)'",
				gpuRuntimeComposeVersion, gpuRuntimeComposeVersion,
			),
			Sudo: true,
		},
		utils.PulumiDependsOn(dockerInstall, jqInstall, ecrHelperInstall),
	)
	if err != nil {
		return nil, err
	}

	return composeInstall, nil
}
