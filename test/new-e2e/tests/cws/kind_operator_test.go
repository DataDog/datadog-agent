// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	scenkind "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awskubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"
	"github.com/google/uuid"
)

func TestKindOperatorSuite(t *testing.T) {
	osDesc, err := platforms.BuildOSDescriptor(fmt.Sprintf("%s/%s/%s", osPlatform, osArch, osVersion))
	if err != nil {
		t.Fatalf("failed to build os descriptor: %v", err)
	}

	ddHostname := fmt.Sprintf("%s-%s", k8sHostnamePrefix, uuid.NewString()[:4])
	customDDA := agentwithoperatorparams.DDAConfig{
		Name: "cws-enabled",
		YamlConfig: fmt.Sprintf(`
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
spec:
  global:
    kubelet:
      tlsVerify: false
  override:
    nodeAgent:
      env:
      - name: DD_HOSTNAME
        value: %s
  features:
    cws:
      enabled: true
`, ddHostname),
	}

	e2e.Run(t, &kindSuite{}, e2e.WithProvisioner(
		awskubernetes.Provisioner(
			awskubernetes.WithRunOptions(
				scenkind.WithVMOptions(
					ec2.WithOS(osDesc),
				),
				scenkind.WithoutFakeIntake(),
				scenkind.WithDeployOperator(),
				scenkind.WithOperatorDDAOptions([]agentwithoperatorparams.Option{
					agentwithoperatorparams.WithDDAConfig(customDDA),
				}...),
			),
		),
	))
}
