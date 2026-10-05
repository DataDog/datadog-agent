// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otlpingest contains e2e OTLP Ingest tests
package otlpingest

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"

	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type otlpIngestLogsTagsAsDDTagsTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// TestOTLPIngestLogsTagsAsDDTags validates that the infraattributes
// processor's logs_tags_as_ddtags option (exposed as
// otlp_config.logs.infra_attributes.tags_as_ddtags) turns custom
// tagger-derived tags (e.g. from kubernetesResourcesLabelsAsTags) into real
// Datadog log tags instead of log attributes, for core-agent OTLP ingestion.
func TestOTLPIngestLogsTagsAsDDTags(t *testing.T) {
	values := `
datadog:
  otlp:
    receiver:
      protocols:
        grpc:
          enabled: true
    logs:
      enabled: true
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
agents:
  containers:
    agent:
      env:
        - name: DD_OTLP_CONFIG_LOGS_INFRA_ATTRIBUTES_TAGS_AS_DDTAGS
          value: 'true'
`
	t.Parallel()
	e2e.Run(t, &otlpIngestLogsTagsAsDDTagsTestSuite{}, e2e.WithProvisioner(
		provkindvm.Provisioner(provkindvm.WithRunOptions(scenkindvm.WithAgentOptions(kubernetesagentparams.WithHelmValues(values))))),
	)
}

var otlpIngestLogsTagsAsDDTagsParams = utils.IAParams{
	InfraAttributes:  true,
	LogsTagsAsDDTags: true,
}

func (s *otlpIngestLogsTagsAsDDTagsTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *otlpIngestLogsTagsAsDDTagsTestSuite) TestOTLPLogs() {
	utils.TestLogs(s, otlpIngestLogsTagsAsDDTagsParams)
}
