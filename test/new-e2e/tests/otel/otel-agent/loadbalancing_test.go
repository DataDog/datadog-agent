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

type loadBalancingTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/loadbalancing.yml
var loadBalancingConfig string

func TestOTelAgentLoadBalancing(t *testing.T) {
	values := `
datadog:
  otelCollector:
    useStandaloneImage: false
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
`
	t.Parallel()
	e2e.Run(t, &loadBalancingTestSuite{},
		e2e.WithSkipDeleteOnFailure(), // DEBUG: Skip delete on failure to keep the cluster alive for investigation
		e2e.WithProvisioner(
			provkindvm.Provisioner(
				provkindvm.WithRunOptions(
					scenkindvm.WithAgentOptions(
						kubernetesagentparams.WithHelmValues(values),
						kubernetesagentparams.WithOTelAgent(),
						kubernetesagentparams.WithOTelConfig(loadBalancingConfig),
					),
				),
			),
		),
	)
}

func (s *loadBalancingTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, "calendar-rest-go-1")
	utils.TestCalendarApp(s, false, "calendar-rest-go-2")
	utils.TestCalendarApp(s, false, "calendar-rest-go-3")
	utils.TestCalendarApp(s, false, "calendar-rest-go-4")
}

func (s *loadBalancingTestSuite) TestLoadBalancing() {
	utils.TestLoadBalancing(s)
}
