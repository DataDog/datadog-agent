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

// k8sInfraModeSuite covers the mark on the payloads the orchestrator check
// emits. The host-side coverage of the same feature (host tags and metrics)
// lives in test/new-e2e/tests/agent-runtimes/infra-mode, and the containers the
// Agent reports are covered by TestInfraModeSuite in
// test/new-e2e/tests/process.
//
// It is a separate entry point rather than a set of methods on k8sSuite because
// infrastructure_mode is Agent-wide: setting it on the shared suite would run
// every existing assertion there under a non-default configuration.
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
