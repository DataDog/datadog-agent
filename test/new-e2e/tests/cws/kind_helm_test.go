// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	scenkind "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	awskubernetes "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-platform/platforms"
	"github.com/google/uuid"
)

// Depending on the pulumi version used to run these tests, the following values may not be properly merged with the default values defined in the test-infra-definitions repository.
// This PR https://github.com/pulumi/pulumi-kubernetes/pull/2963 should fix this issue upstream.
const valuesFmt = `
datadog:
  envDict:
    DD_HOSTNAME: "%s"
  securityAgent:
    runtime:
      enabled: true
      useSecruntimeTrack: false
agents:
  volumes:
    - name: host-root-proc
      hostPath:
        path: /host/proc
  volumeMounts:
    - name: host-root-proc
      mountPath: /host/root/proc
  containers:
    systemProbe:
      env:
        - name: HOST_PROC
          value: "/host/root/proc"
`

func TestKindHelmSuite(t *testing.T) {
	osDesc, err := platforms.BuildOSDescriptor(fmt.Sprintf("%s/%s/%s", osPlatform, osArch, osVersion))
	if err != nil {
		t.Fatalf("failed to build os descriptor: %v", err)
	}

	ddHostname := fmt.Sprintf("%s-%s", k8sHostnamePrefix, uuid.NewString()[:4])
	values := fmt.Sprintf(valuesFmt, ddHostname)
	t.Logf("Running testsuite with DD_HOSTNAME=%s", ddHostname)
	e2e.Run(t, &kindSuite{ddHostname: ddHostname},
		e2e.WithProvisioner(
			awskubernetes.Provisioner(
				awskubernetes.WithRunOptions(
					scenkind.WithVMOptions(
						ec2.WithOS(osDesc),
					),
					scenkind.WithoutFakeIntake(),
					scenkind.WithAgentOptions(
						kubernetesagentparams.WithHelmValues(values),
					),
				),
			),
		),
	)
}
