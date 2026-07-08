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

type headBasedSamplingTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/sampling-head-based.yml
var headBasedSamplingConfig string

// TestOTelAgentHeadBasedSampling verifies that the Datadog connector scales APM
// stats up by the head-based sampling weight encoded in the W3C tracestate, by
// running an unsampled baseline connector alongside a sampler+connector branch
// and asserting their Hits match. See utils.TestHeadBasedSamplingScaling.
func TestOTelAgentHeadBasedSampling(t *testing.T) {
	values := `
datadog:
  otelCollector:
    useStandaloneImage: false
`
	t.Parallel()
	e2e.Run(t, &headBasedSamplingTestSuite{}, e2e.WithProvisioner(
		provkindvm.Provisioner(provkindvm.WithRunOptions(
			scenkindvm.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(values),
				kubernetesagentparams.WithOTelAgent(),
				kubernetesagentparams.WithOTelConfig(headBasedSamplingConfig),
			),
		))),
	)
}

func (s *headBasedSamplingTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *headBasedSamplingTestSuite) TestHeadBasedSamplingScaling() {
	utils.TestHeadBasedSamplingScaling(s)
}
