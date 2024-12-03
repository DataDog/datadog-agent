// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"testing"

	tifeks "github.com/DataDog/test-infra-definitions/scenarios/aws/eks"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
)

type eksSuite struct {
	k8sSuite
}

func TestEKSSuite(t *testing.T) {
	e2e.Run(t, &eksSuite{}, e2e.WithProvisioner(awskubernetes.EKSProvisioner(
		awskubernetes.WithEKSOptions(
			tifeks.WithLinuxNodeGroup(),
			tifeks.WithWindowsNodeGroup(),
			tifeks.WithBottlerocketNodeGroup(),
			tifeks.WithLinuxARMNodeGroup(),
		),
		awskubernetes.WithDeployDogstatsd(),
		awskubernetes.WithDeployTestWorkload(),
	)))
}

func (suite *eksSuite) SetupSuite() {
	suite.k8sSuite.SetupSuite()
	suite.Fakeintake = suite.Env().FakeIntake.Client()
}
