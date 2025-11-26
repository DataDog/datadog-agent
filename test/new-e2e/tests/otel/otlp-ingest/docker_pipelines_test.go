// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otlpingest

import (
	_ "embed"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

type otlpIngestDockerTestSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

//go:embed compose/otlp_ingest_compose.yaml
var otlpIngestCompose string

func TestOTLPIngestDocker(t *testing.T) {
	t.Parallel()
	e2e.Run(t,
		&otlpIngestDockerTestSuite{},
		e2e.WithProvisioner(
			awsdocker.Provisioner(
				awsdocker.WithRunOptions(
					ec2docker.WithAgentOptions(
						dockeragentparams.WithLogs(),
						dockeragentparams.WithAgentServiceEnvVariable("DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_GRPC_ENDPOINT", pulumi.StringPtr("0.0.0.0:4317")),
						dockeragentparams.WithAgentServiceEnvVariable("DD_OTLP_CONFIG_RECEIVER_PROTOCOLS_HTTP_ENDPOINT", pulumi.StringPtr("0.0.0.0:4318")),
						dockeragentparams.WithAgentServiceEnvVariable("DD_LOGS_ENABLED", pulumi.StringPtr("true")),
						dockeragentparams.WithAgentServiceEnvVariable("DD_OTLP_CONFIG_LOGS_ENABLED", pulumi.StringPtr("true")),
						dockeragentparams.WithAgentServiceEnvVariable("DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL", pulumi.StringPtr("false")),
						dockeragentparams.WithAgentServiceEnvVariable("DD_OTLP_CONFIG_METRICS_RESOURCE_ATTRIBUTES_AS_TAGS", pulumi.StringPtr("true")),
						dockeragentparams.WithExtraComposeManifest("calendar-rest-go", pulumi.String(strings.ReplaceAll(otlpIngestCompose, "{APPS_VERSION}", apps.Version))),
					),
				),
			),
		),
	)
}

func (s *otlpIngestDockerTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	// SetupSuite needs to defer CleanupOnSetupFailure() if what comes after BaseSuite.SetupSuite() can fail.
	defer s.CleanupOnSetupFailure()

	utils.TestCalendarAppDocker(s)
}

func (s *otlpIngestDockerTestSuite) TestOTLPTraces() {
	utils.TestTracesDocker(s)
}

func (s *otlpIngestDockerTestSuite) TestOTLPMetrics() {
	utils.TestMetricsDocker(s)
}

func (s *otlpIngestDockerTestSuite) TestOTLPLogs() {
	utils.TestLogsDocker(s)
}
