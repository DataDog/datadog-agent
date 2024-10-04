// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
)

type eksSuite struct {
	k8sSuite
}

func TestEKSSuite(t *testing.T) {
	e2e.Run(t, &eksSuite{}, e2e.WithProvisioner(awskubernetes.EKSProvisioner(
		awskubernetes.WithEKSLinuxNodeGroup(),
		awskubernetes.WithEKSWindowsNodeGroup(),
		awskubernetes.WithEKSBottlerocketNodeGroup(),
		awskubernetes.WithDeployDogstatsd(),
		awskubernetes.WithDeployTestWorkload(),
		awskubernetes.WithEKSLinuxARMNodeGroup(),
	)))
}

func (s *eksSuite) SetupSuite() {
	s.k8sSuite.SetupSuite()
	s.Fakeintake = s.Env().FakeIntake.Client()
}
