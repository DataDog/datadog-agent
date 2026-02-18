// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package sbom

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	sceneks "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	proveks "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/eks"
)

type eksSuite struct {
	k8sSuite
}

func TestSBOMEKSSuite(t *testing.T) {
	newProvisioner := func(helmValues string) provisioners.Provisioner {
		return proveks.Provisioner(
			proveks.WithRunOptions(
				sceneks.WithEKSOptions(
					sceneks.WithLinuxNodeGroup(),
					sceneks.WithBottlerocketNodeGroup(),
					sceneks.WithLinuxARMNodeGroup(),
				),
				sceneks.WithDeployDogstatsd(),
				sceneks.WithDeployTestWorkload(),
				sceneks.WithAgentOptions(
					kubernetesagentparams.WithDualShipping(),
					kubernetesagentparams.WithHelmValues(helmValues),
				),
				sceneks.WithDeployArgoRollout(),
			),
		)
	}
	e2e.Run(t, &eksSuite{k8sSuite{newProvisioner: newProvisioner, skipModes: []string{"overlayfs"}}}, e2e.WithProvisioner(newProvisioner("")))
}

func (suite *eksSuite) SetupSuite() {
	suite.k8sSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
}
