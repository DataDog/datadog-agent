// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelagent contains e2e otel agent tests
package otelagent

import (
	_ "embed"
	"testing"

	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type hostmetricsreceiverTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/hostmetricsreceiver.yml
var hostmetricsreceiverConfig string

func TestOTelAgentHostmetricsReceiver(t *testing.T) {
	values := enableOTELAgentonfig(`
datadog:
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
`)
	t.Parallel()
	e2e.Run(t, &hostmetricsreceiverTestSuite{},
		e2e.WithProvisioner(
			awskubernetes.KindProvisioner(
				awskubernetes.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(values),
					kubernetesagentparams.WithOTelAgent(),
					kubernetesagentparams.WithOTelConfig(hostmetricsreceiverConfig),
				))))
}

func (s *hostmetricsreceiverTestSuite) TestOTelAgentInstalled() {
	utils.TestOTelAgentInstalled(s)
}

func (s *hostmetricsreceiverTestSuite) TestHostMetrics() {
	utils.TestHostMetrics(s)
}
