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
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type noDDExporterTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/no-dd-exporter.yml
var noDDExporterConfig string

func TestOTelAgentWithNoDDExporter(t *testing.T) {
	values := `
datadog:
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
`
	t.Parallel()
	e2e.Run(t, &noDDExporterTestSuite{},
		e2e.WithProvisioner(
			awskubernetes.KindProvisioner(
				awskubernetes.WithAgentOptions(
					kubernetesagentparams.WithoutDualShipping(),
					kubernetesagentparams.WithHelmValues(values),
					kubernetesagentparams.WithOTelAgent(),
					kubernetesagentparams.WithOTelConfig(noDDExporterConfig),
				))))
}

func (s *noDDExporterTestSuite) TestOTelAgentInstalled() {
	utils.TestOTelAgentInstalled(s)
}

func (s *noDDExporterTestSuite) TestFlare() {
	expectedContents := []string{
		"otel-agent",
		"ddflare/dd-autoconfigured:",
		"health_check/dd-autoconfigured:",
		"pprof/dd-autoconfigured:",
		"zpages/dd-autoconfigured:",
		"infraattributes/dd-autoconfigured:",
		"prometheus/dd-autoconfigured:",
	}
	utils.TestOTelFlare(s, expectedContents)
}
