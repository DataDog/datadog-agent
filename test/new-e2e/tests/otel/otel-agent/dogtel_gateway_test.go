// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelagent contains e2e otel agent tests
package otelagent

import (
	"context"
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	pulumicorev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	k8sclient "k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	otelstandalone "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/otel-standalone"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	provlocal "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

//go:embed config/dogtel-standalone-otlp.yml
var dogtelGatewayLeafConfig string

// gatewayNamespace is a namespace distinct from the standalone otel-agent leaf's
// "datadog" namespace. Both otelstandalone.K8sAppDefinition and the Helm agent
// installation create an image-pull secret named after their namespace whenever
// a private registry is configured (true in CI); reusing "datadog" for both
// would make them collide on the same Pulumi resource name (see coreAgentNamespace
// in dogtel_standalone_test.go for the same constraint in the coexist suite).
//
// This also means utils.TestOTelGatewayInstalled and utils.TestOTelGatewayFlareCmd,
// which hardcode namespace "datadog" for the gateway pod lookup, cannot be reused
// here — this suite checks gateway health via payload assertions instead.
const gatewayNamespace = "datadog-gateway"

// dogtelGatewayHelmValues forces the Helm chart's ddot-collector-image helper
// off its useStandaloneImage default (true), which otherwise runs a
// semverCompare against the agent image tag; that check panics on floating
// dev tags such as the default nightly-full-main-jmx. It also disables the
// core Agent's own container log tailing, since the gateway's logs come
// through the OTLP pipeline already — leaving file tailing on double-ingests
// the calendar app's stdout, and that copy lacks the OTLP-derived tags
// (e.g. custom.attribute, k8s.container.name) that utils.TestLogs asserts on
// (mirrors gatewayTestSuite's values in gateway_test.go). Finally, it disables
// the core node-agent DaemonSet entirely (agents.enabled: false): this suite
// is meant to exercise the gateway collector on its own, and TestNoCoreAgent
// asserts this Helm release never runs a core Agent pod alongside it.
const dogtelGatewayHelmValues = `
datadog:
  otelCollector:
    useStandaloneImage: false
  logs:
    containerCollectAll: false
    containerCollectUsingFiles: false
agents:
  enabled: false
`

// dogtelGatewayTestSuite verifies a standalone otel-agent (DD_OTEL_STANDALONE=true,
// dogtelextension enabled) configured with an OTLP exporter — instead of the datadog
// exporter used by dogtelStandaloneTestSuite — forwarding traces, metrics, and logs
// to a separate, Helm-deployed OTel gateway collector, which in turn ships them to
// fakeintake. This covers the "leaf otel-agent -> gateway otel-agent -> intake"
// topology, combining the standalone/dogtel coverage of dogtelStandaloneTestSuite
// with the gateway coverage of gatewayTestSuite.
type dogtelGatewayTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// dogtelGatewayProvisioner deploys the standalone otel-agent leaf (OTLP exporter,
// no datadog exporter) alongside a Helm-deployed OTel gateway in a distinct
// namespace, mirroring dogtelCoexistProvisioner's collision-avoidance pattern.
func dogtelGatewayProvisioner() provisioners.TypedProvisioner[environments.Kubernetes] {
	deployFn := func(e config.Env, kubeProvider *kubernetes.Provider, fi *fakeintakeComp.Fakeintake) (*agent.KubernetesAgent, error) {
		// fakeIntake is passed as nil so K8sAppDefinition does not merge a
		// datadog exporter endpoint into a config that has none; the liveness
		// metric still needs a route to fakeintake via the agent serializer,
		// so DD_DD_URL is added manually below.
		return otelstandalone.K8sAppDefinition(e, kubeProvider, "datadog", dogtelGatewayLeafConfig, nil,
			otelstandalone.WithExtraEnvVars(&pulumicorev1.EnvVarArgs{
				Name:  pulumi.String("DD_DD_URL"),
				Value: fi.URL,
			}),
		)
	}
	if isKindLocal() {
		return provlocal.Provisioner(
			provlocal.WithStandaloneOTelAgent(deployFn),
			provlocal.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(dogtelGatewayHelmValues),
				kubernetesagentparams.WithNamespace(gatewayNamespace),
				kubernetesagentparams.WithOTelAgent(),
				kubernetesagentparams.WithOTelGatewayConfig(gatewayConfig),
				kubernetesagentparams.WithOTelAgentGateway(),
			),
		)
	}
	return provkindvm.Provisioner(
		provkindvm.WithRunOptions(
			scenkindvm.WithStandaloneOTelAgent(deployFn),
			scenkindvm.WithAgentOptions(
				kubernetesagentparams.WithHelmValues(dogtelGatewayHelmValues),
				kubernetesagentparams.WithNamespace(gatewayNamespace),
				kubernetesagentparams.WithOTelAgent(),
				kubernetesagentparams.WithOTelGatewayConfig(gatewayConfig),
				kubernetesagentparams.WithOTelAgentGateway(),
			),
		),
	)
}

// TestDogtelStandaloneGateway is the entry point for the suite.
func TestDogtelStandaloneGateway(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &dogtelGatewayTestSuite{},
		e2e.WithProvisioner(dogtelGatewayProvisioner()),
		e2e.WithCoverageRequired(map[string]bool{
			// env.Agent resolves to the standalone otel-agent leaf (provisioners
			// export it after the Helm agent), so requiring "agent" coverage
			// would exec the core agent's coverage command inside a pod that
			// only has an otel-agent container.
			"agent":      false,
			"otel-agent": true,
		}),
	)
}

var dogtelGatewayParams = utils.IAParams{
	InfraAttributes: true,
	EKS:             false,
	Cardinality:     types.LowCardinality,
	// The standalone otel-agent leaf has no Helm chart / Cluster Agent, so
	// kubernetesResourcesLabelsAsTags is never configured for it.
	SkipCustomLabelTag: true,
}

func (s *dogtelGatewayTestSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	defer s.CleanupOnSetupFailure()
	// Verify the liveness metric BEFORE TestCalendarApp flushes the aggregators.
	// The metric is emitted once by dogtelextension.Start() at startup; it must be
	// captured here before FlushServerAndResetAggregators() clears it.
	s.T().Log("Waiting for dogtel liveness metric before aggregator flush")
	require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		metrics, err := s.Env().FakeIntake.Client().FilterMetrics(utils.DogtelLivenessMetricName)
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 5*time.Minute, 10*time.Second, "dogtel liveness metric not received after agent startup")
	utils.TestCalendarApp(s, false, utils.CalendarService)
}

// TestDogtelLivenessMetric verifies that the extension reports itself running.
// The metric is checked in SetupSuite before the first aggregator flush.
func (s *dogtelGatewayTestSuite) TestDogtelLivenessMetric() {
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(utils.DogtelLivenessMetricName)
	require.NoError(s.T(), err)
	if len(metrics) > 0 {
		s.T().Log("Got dogtel liveness metric:", metrics[0])
		require.NotEmpty(s.T(), metrics[0].Points)
		assert.Equal(s.T(), 1.0, metrics[0].Points[0].Value, "otel.dogtel_extension.running should always be 1.0")
	} else {
		s.T().Log("Liveness metric was verified in SetupSuite; not yet re-emitted since last flush (no heartbeat)")
	}
}

// TestDogtelTaggerServerRunning confirms the tagger gRPC server started by the
// leaf's dogtelextension is listening on port 15555.
func (s *dogtelGatewayTestSuite) TestDogtelTaggerServerRunning() {
	utils.TestDogtelTaggerServerRunning(s, 15555)
}

// TestOTLPTraces verifies traces sent by the leaf reach the gateway and, from
// there, fakeintake, carrying the gateway tag the gateway's trace-agent sets on
// the AgentPayload (mirrors gatewayTestSuite.TestOTLPTraces).
func (s *dogtelGatewayTestSuite) TestOTLPTraces() {
	utils.TestTraces(s, dogtelGatewayParams)

	traces, err := s.Env().FakeIntake.Client().GetTraces()
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), traces)
	for _, trace := range traces {
		assert.NotEmpty(s.T(), trace.HostName, "agent hostname should not be empty in gateway mode")
		assert.Equal(s.T(), "true", trace.Tags["_dd.otel.gateway"], "gateway tag should be set on AgentPayload")
	}
}

// TestOTLPMetrics verifies metrics sent by the leaf reach the gateway and fakeintake.
func (s *dogtelGatewayTestSuite) TestOTLPMetrics() {
	utils.TestMetrics(s, dogtelGatewayParams)
}

// TestOTLPLogs verifies logs sent by the leaf reach the gateway and fakeintake.
func (s *dogtelGatewayTestSuite) TestOTLPLogs() {
	utils.TestLogs(s, dogtelGatewayParams)
}

// TestHosts verifies traces, metrics, and logs report consistent hostnames
// end-to-end through the leaf -> gateway -> fakeintake path.
func (s *dogtelGatewayTestSuite) TestHosts() {
	utils.TestHosts(s)
}

// TestNoCoreAgent verifies neither side of this topology runs a core Datadog
// Agent pod: the leaf is a bare DaemonSet manifest containing only an
// otel-agent container (otelstandalone.K8sAppDefinition never creates a core
// Agent), and the gateway's Helm release sets agents.enabled: false in
// dogtelGatewayHelmValues so only the otelAgentGateway Deployment is created.
func (s *dogtelGatewayTestSuite) TestNoCoreAgent() {
	assertNoPodWithAppLabel(s.T(), s.Env().KubernetesCluster.Client(), "datadog", coreAgentPodLabel)
	assertNoPodWithAppLabel(s.T(), s.Env().KubernetesCluster.Client(), gatewayNamespace, coreAgentPodLabel)
}

func assertNoPodWithAppLabel(t *testing.T, client k8sclient.Interface, namespace, appLabel string) {
	res, err := client.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", appLabel).String(),
	})
	require.NoError(t, err)
	assert.Empty(t, res.Items, "expected no core agent pod (app=%q) in namespace %q", appLabel, namespace)
}
