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

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type logsTagsAsDDTagsTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

//go:embed config/logs_tags_as_ddtags.yml
var logsTagsAsDDTagsConfig string

// TestOTelAgentLogsTagsAsDDTags validates that the infraattributes
// processor's logs_tags_as_ddtags option turns custom tagger-derived tags
// (e.g. from kubernetesResourcesLabelsAsTags) into real Datadog log tags
// instead of log attributes, for DDOT (standalone OTel collector).
func TestOTelAgentLogsTagsAsDDTags(t *testing.T) {
	values := `
datadog:
  otelCollector:
    useStandaloneImage: false
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
`
	t.Parallel()
	e2e.Run(t, &logsTagsAsDDTagsTestSuite{},
		e2e.WithProvisioner(provkindvm.Provisioner(
			provkindvm.WithRunOptions(
				scenkindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(values),
					kubernetesagentparams.WithOTelAgent(),
					kubernetesagentparams.WithOTelConfig(logsTagsAsDDTagsConfig),
				),
			),
		)),
	)
}

var logsTagsAsDDTagsParams = utils.IAParams{
	InfraAttributes:  true,
	Cardinality:      types.HighCardinality,
	LogsTagsAsDDTags: true,
}

func (s *logsTagsAsDDTagsTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarApp(s, false, utils.CalendarService)
}

func (s *logsTagsAsDDTagsTestSuite) TestOTLPLogs() {
	utils.TestLogs(s, logsTagsAsDDTagsParams)
}
