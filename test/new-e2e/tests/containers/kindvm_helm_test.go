// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
)

func TestKindHelmSuite(t *testing.T) {
	helmValues := `
clusterAgent:
    envDict:
        DD_CSI_ENABLED: "true"
`

	e2e.Run(t, &kindSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(
		awskubernetes.WithEC2VMOptions(
			ec2.WithInstanceType("t3.xlarge"),
		),
		awskubernetes.WithFakeIntakeOptions(fakeintake.WithMemory(2048)),
		awskubernetes.WithDeployDogstatsd(),
		awskubernetes.WithDeployTestWorkload(),
		awskubernetes.WithAgentOptions(
			kubernetesagentparams.WithDualShipping(),
			kubernetesagentparams.WithHelmValues(helmValues),
		),
	)))

	e2e.Run(t, &kindSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(
		awskubernetes.WithEC2VMOptions(
			ec2.WithInstanceType("t3.xlarge"),
		),
		awskubernetes.WithFakeIntakeOptions(fakeintake.WithMemory(2048)),
		awskubernetes.WithDeployDogstatsd(),
		awskubernetes.WithDeployTestWorkload(),
		awskubernetes.WithAgentOptions(
			kubernetesagentparams.WithDualShipping(),
			kubernetesagentparams.WithHelmValues(helmValues),
		),
	)))
}
