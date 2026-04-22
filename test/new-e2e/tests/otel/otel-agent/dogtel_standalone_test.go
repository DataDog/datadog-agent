// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package otelagent contains e2e otel agent tests
package otelagent

import (
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"

	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	otelstandalone "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/otel-standalone"
	scenkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	provkindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	provlocal "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/local/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/otel/utils"
)

//go:embed config/dogtel-standalone.yml
var dogtelStandaloneConfig string

// dogtelStandaloneTestSuite tests the dogtelextension running in standalone mode
// (DD_OTEL_STANDALONE=true). In this mode the extension starts its own workloadmeta
// store, tagger, and tagger gRPC server, providing Kubernetes infrastructure
// attribute enrichment independently of a co-located core Datadog Agent.
type dogtelStandaloneTestSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

// dogtelStandaloneProvisioner returns the appropriate provisioner based on the
// E2E_DEV_LOCAL / E2E_PROVISIONER config, mirroring the SSI test pattern.
// - kind-local (or E2E_DEV_LOCAL=true): uses a local KinD cluster
// - default: uses KinD-on-EC2 (AWS)
func dogtelStandaloneProvisioner() provisioners.TypedProvisioner[environments.Kubernetes] {
	deployFn := func(e config.Env, kubeProvider *kubernetes.Provider, fi *fakeintakeComp.Fakeintake) (*agent.KubernetesAgent, error) {
		return otelstandalone.K8sAppDefinition(e, kubeProvider, "datadog", dogtelStandaloneConfig, fi)
	}
	if isKindLocal() {
		return provlocal.Provisioner(
			provlocal.WithStandaloneOTelAgent(deployFn),
		)
	}
	return provkindvm.Provisioner(
		provkindvm.WithRunOptions(
			scenkindvm.WithStandaloneOTelAgent(deployFn),
		),
	)
}

// isKindLocal returns true when E2E_DEV_LOCAL=true or E2E_PROVISIONER=kind-local.
func isKindLocal() bool {
	devLocal, err := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.DevLocal, false)
	if err == nil && devLocal {
		return true
	}
	provisioner, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.Provisioner, "")
	return err == nil && strings.EqualFold(provisioner, "kind-local")
}

// TestDogtelStandalone is the entry point for the suite.
// It provisions a KindVM cluster and deploys the otel-agent as a standalone
// DaemonSet (no Helm chart, no core agent) with DD_OTEL_STANDALONE=true.
// The dogtel-standalone OTel config enables the dogtelextension with a tagger
// gRPC server on port 15555.
//
// The name is intentionally short (≤20 lowercase chars) to prevent Kubernetes
// from truncating pod names: deployment name = "calendar-rest-go-" + lowercase(TestName).
// Kubernetes truncates pod generateName at 57 chars, so RS names > 57 chars cause
// pod names to omit part of the RS hash, breaking testInfraTags assertions.
func TestDogtelStandalone(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &dogtelStandaloneTestSuite{},
		e2e.WithProvisioner(dogtelStandaloneProvisioner()),
	)
}

var dogtelParams = utils.IAParams{
	InfraAttributes: true,
	EKS:             false,
	Cardinality:     types.LowCardinality,
}

func (s *dogtelStandaloneTestSuite) SetupSuite() {
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

// TestDogtelAgentInstalled checks the otel-agent pod is running with the dogtelextension.
func (s *dogtelStandaloneTestSuite) TestDogtelAgentInstalled() {
	utils.TestOTelAgentInstalled(s)
}

// TestDogtelLivenessMetric verifies that the extension reports itself running.
// The metric is checked in SetupSuite before the first aggregator flush.
// If the binary emits the metric periodically (heartbeat), this test will also
// catch a post-flush emission; otherwise it passes because SetupSuite already verified it.
func (s *dogtelStandaloneTestSuite) TestDogtelLivenessMetric() {
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(utils.DogtelLivenessMetricName)
	require.NoError(s.T(), err)
	if len(metrics) > 0 {
		// Metric present (heartbeat or not yet flushed).
		s.T().Log("Got dogtel liveness metric:", metrics[0])
		require.NotEmpty(s.T(), metrics[0].Points)
		assert.Equal(s.T(), 1.0, metrics[0].Points[0].Value, "otel.dogtel_extension.running should always be 1.0")
	} else {
		// Already flushed since SetupSuite; the metric was verified there.
		s.T().Log("Liveness metric was verified in SetupSuite; not yet re-emitted since last flush (no heartbeat)")
	}
}

// TestDogtelTaggerServerRunning confirms the tagger gRPC server is bound to
// port 15555 inside the otel-agent container, as configured in dogtel-standalone.yml.
// The server is started by dogtelextension.startTaggerServer() and exposes the
// workloadmeta-backed tagger to remote clients over mTLS.
func (s *dogtelStandaloneTestSuite) TestDogtelTaggerServerRunning() {
	utils.TestDogtelTaggerServerRunning(s, 15555)
}

// TestDogtelOTLPTraces verifies OTLP traces are enriched with Kubernetes workloadmeta
// tags (kube_deployment, pod_name, kube_namespace, etc.) via the infraattributes processor.
// In standalone mode these tags come from the tagger started by dogtelextension, which
// subscribes to the local workloadmeta store that watches the Kubernetes API.
func (s *dogtelStandaloneTestSuite) TestDogtelOTLPTraces() {
	utils.TestTraces(s, dogtelParams)
}

// TestDogtelOTLPMetrics verifies OTLP metrics carry Kubernetes workloadmeta tags.
func (s *dogtelStandaloneTestSuite) TestDogtelOTLPMetrics() {
	utils.TestMetrics(s, dogtelParams)
}

// TestDogtelOTLPLogs verifies OTLP logs carry Kubernetes workloadmeta tags.
func (s *dogtelStandaloneTestSuite) TestDogtelOTLPLogs() {
	utils.TestLogs(s, dogtelParams)
}

// TestDogtelHosts verifies that traces, metrics, and logs all report the same
// hostname, which in standalone mode is resolved by dogtelextension's hostname
// component (backed by the k8s node name from workloadmeta).
func (s *dogtelStandaloneTestSuite) TestDogtelHosts() {
	utils.TestHosts(s)
}
