// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// GPU images pre-bake Docker tooling; Kind still requires a runtime install.

package gpu

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	componentsos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	componentsremote "github.com/DataDog/datadog-agent/test/e2e-framework/components/remote"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// installGPUKind installs kind at runtime, mirroring what the e2e AMIs ship
// preinstalled (see the "24-04-e2e-kind" AMI and the kindvm scenario).
// nvkind drives kind under the hood, so the GPU VM needs it on PATH. Wire its
// returned Command as a Pulumi dependency on nvidia.NewKindCluster.
//
// TODO: remove once the GPU e2e AMI variants ship kind pre-baked, along with
// the rest of this file (ACIX-1305).
func installGPUKind(awsEnv *aws.Environment, host *componentsremote.Host) (command.Command, error) {
	kindVersionConfig, err := kubeComp.GetKindVersionConfig(awsEnv.KubernetesVersion())
	if err != nil {
		return nil, err
	}

	kindArch := host.OS.Descriptor().Architecture
	if kindArch == componentsos.AMD64Arch {
		kindArch = "amd64"
	}

	return host.OS.Runner().Command(
		awsEnv.Namer.ResourceName("gpu-kind-install"),
		&command.Args{
			Create: pulumi.Sprintf(`curl --retry 10 -fsSLo ./kind "https://kind.sigs.k8s.io/dl/%s/kind-linux-%s" && sudo install kind /usr/local/bin/kind`, kindVersionConfig.KindVersion, kindArch),
		},
	)
}
