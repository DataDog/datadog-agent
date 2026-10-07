// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package orchestrator

import (
	_ "embed"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentmodel "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	scenariokindvm "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awskindvm "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	fakeintake "github.com/DataDog/datadog-agent/test/fakeintake/client"
)

//go:embed agent_infra_mode_values.yaml
var agentInfraModeValues string

// infrastructureModeTag is the mark an Agent running in cloud_cost_only mode
// stamps on the payloads it produces, so that each backend consumer can tell
// them from the payloads of a fully monitored host.
const infrastructureModeTag = "infra_mode:cloud_cost_only"

// k8sInfraModeSuite covers the mark on the payloads the Cluster Agent emits:
// orchestrator resources, their manifests, and Kubernetes events. Host tags,
// metrics, and containers are covered by tests/agent-runtimes/infra-mode and
// tests/process.
//
// It is a separate entry point because infrastructure_mode is Agent-wide:
// setting it on k8sSuite would run every assertion there under a non-default
// configuration.
type k8sInfraModeSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestKindInfraModeSuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &k8sInfraModeSuite{},
		// The demo workload is deliberately left out: the assertions below only
		// need some pod to exist, which the cluster's own namespaces provide.
		// Deploying it grows the payload volume past the point where a
		// fakeintake read completes within the client's timeout.
		e2e.WithProvisioner(awskindvm.Provisioner(
			awskindvm.WithRunOptions(
				scenariokindvm.WithAgentOptions(
					kubernetesagentparams.WithHelmValues(agentInfraModeValues),
				),
			),
		)),
	)
}

// TestPodResourceCarriesInfraMode covers the resource payload, which the
// Cluster Agent produces through the orchestrator check.
func (suite *k8sInfraModeSuite) TestPodResourceCarriesInfraMode() {
	expectAtLeastOneResource{
		filter: &fakeintake.PayloadFilter{ResourceType: agentmodel.TypeCollectorPod},
		test: func(payload *aggregator.OrchestratorPayload) bool {
			return slices.Contains(payload.Tags, infrastructureModeTag)
		},
		message: "find a pod payload tagged " + infrastructureModeTag,
		timeout: defaultTimeout,
	}.Assert(suite.T(), suite.Env().FakeIntake.Client())
}

// TestManifestEnvelopeCarriesInfraMode covers the manifest envelope, which
// carries its own tagset rather than inheriting the one on the resource.
func (suite *k8sInfraModeSuite) TestManifestEnvelopeCarriesInfraMode() {
	suite.EventuallyWithT(func(c *assert.CollectT) {
		payloads, err := suite.Env().FakeIntake.Client().GetOrchestratorManifests()
		require.NoError(c, err)

		marked := slices.ContainsFunc(payloads, func(payload *aggregator.OrchestratorManifestPayload) bool {
			return payload.ManifestParentCollector != nil &&
				slices.Contains(payload.ManifestParentCollector.Tags, infrastructureModeTag)
		})
		assert.Truef(c, marked, "no manifest envelope tagged %s among %d payloads", infrastructureModeTag, len(payloads))
	}, defaultTimeout, 15*time.Second)
}

// kubernetesEventSource is the source the kubernetes_apiserver check reports
// events under while kubernetes_events_source_detection stays disabled.
const kubernetesEventSource = "kubernetes"

// TestKubernetesEventCarriesInfraMode covers the bundled Kubernetes events
// path, the one the kubernetes_apiserver check serves by default. Event
// collection is enabled for this suite alone, through
// DD_COLLECT_KUBERNETES_EVENTS in agent_infra_mode_values.yaml.
//
// The events come from the cluster's own activity, so no workload is deployed
// to produce them.
func (suite *k8sInfraModeSuite) TestKubernetesEventCarriesInfraMode() {
	suite.EventuallyWithT(func(c *assert.CollectT) {
		events, err := suite.Env().FakeIntake.Client().FilterEvents(kubernetesEventSource)
		require.NoError(c, err)

		marked := slices.ContainsFunc(events, func(e *aggregator.Event) bool {
			return slices.Contains(e.GetTags(), infrastructureModeTag)
		})
		assert.Truef(c, marked, "no Kubernetes event tagged %s among %d events", infrastructureModeTag, len(events))
	}, defaultTimeout, 15*time.Second)
}
