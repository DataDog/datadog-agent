// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package sbom

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	scenkind "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	provkind "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
)

type kindSuite struct {
	k8sSuite
}

func TestSBOMKindSuite(t *testing.T) {
	newProvisioner := func(helmValues string) provisioners.Provisioner {
		return provkind.Provisioner(
			provkind.WithRunOptions(
				scenkind.WithVMOptions(
					scenec2.WithInstanceType("t3.xlarge"),
				),
				scenkind.WithFakeintakeOptions(
					fakeintake.WithMemory(2048),
				),
				scenkind.WithDeployTestWorkload(),
				scenkind.WithAgentOptions(
					kubernetesagentparams.WithDualShipping(),
					kubernetesagentparams.WithHelmValues(helmValues),
				),
			),
		)
	}
	env := &kindSuite{k8sSuite{newProvisioner: newProvisioner}}
	provisioner := e2e.WithProvisioner(newProvisioner(""))
	e2e.Run(t, env, provisioner)
}

func (suite *kindSuite) SetupSuite() {
	suite.k8sSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
}
