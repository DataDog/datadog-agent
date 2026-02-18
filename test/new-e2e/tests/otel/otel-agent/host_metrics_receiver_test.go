// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelagent contains e2e otel agent tests
package otelagent

import (
	_ "embed"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"

	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type hostmetricsreceiverTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/hostmetricsreceiver.yml
var hostmetricsreceiverConfig string

func TestOTelAgentHostmetricsReceiver(t *testing.T) {
	values := `
datadog:
  otelCollector:
    useStandaloneImage: false
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
`
	t.Parallel()
	e2e.Run(t, &hostmetricsreceiverTestSuite{},
		e2e.WithProvisioner(
			provkindvm.Provisioner(
				provkindvm.WithRunOptions(
					scenkindvm.WithAgentOptions(
						kubernetesagentparams.WithHelmValues(values),
						kubernetesagentparams.WithOTelAgent(),
						kubernetesagentparams.WithOTelConfig(hostmetricsreceiverConfig),
					),
				),
			)),
	)
}

func (s *hostmetricsreceiverTestSuite) TestOTelAgentInstalled() {
	utils.TestOTelAgentInstalled(s)
}

func (s *hostmetricsreceiverTestSuite) TestHostMetrics() {
	utils.TestHostMetrics(s)
}
