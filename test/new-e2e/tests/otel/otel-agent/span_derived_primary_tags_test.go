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

type spanDerivedPrimaryTagsTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/span-derived-primary-tags.yml
var spanDerivedPrimaryTagsConfig string

// TestOTelAgentSpanDerivedPrimaryTags verifies that attribute keys listed under
// the Datadog connector's traces.span_derived_primary_tags are attached to the
// APM stats the connector emits. See utils.TestSpanDerivedPrimaryTags.
func TestOTelAgentSpanDerivedPrimaryTags(t *testing.T) {
	values := `
datadog:
  otelCollector:
    useStandaloneImage: false
`
	t.Parallel()
	e2e.Run(t, &spanDerivedPrimaryTagsTestSuite{}, e2e.WithProvisioner(
		provkindvm.Provisioner(provkindvm.WithRunOptions(
			scenkindvm.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(values),
				kubernetesagentparams.WithOTelAgent(),
				kubernetesagentparams.WithOTelConfig(spanDerivedPrimaryTagsConfig),
			),
		))),
	)
}

func (s *spanDerivedPrimaryTagsTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *spanDerivedPrimaryTagsTestSuite) TestSpanDerivedPrimaryTags() {
	utils.TestSpanDerivedPrimaryTags(s)
}
