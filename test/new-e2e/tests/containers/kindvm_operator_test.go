// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package containers

import (
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
)

func TestKindOperatorSuite(t *testing.T) {
	customDDA := agentwithoperatorparams.DDAConfig{
		Name: "customDDA",
		YamlConfig: `
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
spec:
  global:
    kubelet:
      tlsVerify: false
`,
	}

	e2e.Run(t, &kindSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(
		awskubernetes.WithEC2VMOptions(
			ec2.WithInstanceType("t3.xlarge"),
		),
		awskubernetes.WithFakeIntakeOptions(fakeintake.WithMemory(2048)),
		awskubernetes.WithDeployDogstatsd(),
		awskubernetes.WithDeployTestWorkload(),
		awskubernetes.WithOperator(),
		awskubernetes.WithOperatorDDAOptions([]agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(customDDA),
		}...),
	)))

	e2e.Run(t, &kindSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(
		awskubernetes.WithEC2VMOptions(
			ec2.WithInstanceType("t3.xlarge"),
		),
		awskubernetes.WithFakeIntakeOptions(fakeintake.WithMemory(2048)),
		awskubernetes.WithDeployDogstatsd(),
		awskubernetes.WithDeployTestWorkload(),
		awskubernetes.WithOperatorDDAOptions([]agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(customDDA),
		}...),
	)))
}
