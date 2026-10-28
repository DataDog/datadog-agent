// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package orchestrator

import (
	"context"
	_ "embed"
	"strings"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/dynamic"

	agentmodel "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

//go:embed fixtures/argo_rollout.yaml
var argoRolloutManifest string

var (
	crdGVR     = schema.GroupVersionResource{Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions"}
	rolloutGVR = schema.GroupVersionResource{Group: "argoproj.io", Version: "v1alpha1", Resource: "rollouts"}
)

// TestCRManifOfCRDInstalledAfterAgentStart checks that the built-in CRs of a CRD installed after the Cluster Agent
// started are collected without restarting it.
func (suite *k8sSuite) TestCRManifOfCRDInstalledAfterAgentStart() {
	// A CRD manifest proves the check has run, so the Rollout CRD below is installed after the check prepared its
	// collectors instead of being found by their initial discovery.
	expectAtLeastOneManifest{
		test: func(payload *aggregator.OrchestratorManifestPayload, _ manifest) bool {
			return payload.Type == agentmodel.TypeCollectorManifestCRD
		},
		message: "find a CRD manifest before installing the Rollout CRD",
		timeout: defaultTimeout,
	}.Assert(suite)

	ctx := context.Background()
	client, err := dynamic.NewForConfig(suite.Env().KubernetesCluster.KubernetesClient.K8sConfig)
	require.NoError(suite.T(), err)

	decoder := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(argoRolloutManifest), 4096)
	crd, rollout := &unstructured.Unstructured{}, &unstructured.Unstructured{}
	require.NoError(suite.T(), decoder.Decode(&crd.Object))
	require.NoError(suite.T(), decoder.Decode(&rollout.Object))

	_, err = client.Resource(crdGVR).Create(ctx, crd, metav1.CreateOptions{})
	require.NoError(suite.T(), err)
	// Deleting the CRD deletes its CRs too.
	suite.T().Cleanup(func() {
		_ = client.Resource(crdGVR).Delete(context.Background(), crd.GetName(), metav1.DeleteOptions{})
	})

	// The CR can only be created once the CRD is established.
	suite.EventuallyWithT(func(c *assert.CollectT) {
		_, err := client.Resource(rolloutGVR).Namespace(rollout.GetNamespace()).Create(ctx, rollout, metav1.CreateOptions{})
		assert.NoError(c, err)
	}, 2*time.Minute, 2*time.Second)

	expectAtLeastOneManifest{
		test: func(payload *aggregator.OrchestratorManifestPayload, manif manifest) bool {
			return payload.Type == agentmodel.TypeCollectorManifestCR &&
				manif.APIVersion == "argoproj.io/v1alpha1" &&
				manif.Kind == "Rollout" &&
				manif.Metadata.Name == rollout.GetName()
		},
		message: "find an Argo Rollout manifest CR of a CRD installed after the Cluster Agent started",
		timeout: defaultTimeout,
	}.Assert(suite)
}
