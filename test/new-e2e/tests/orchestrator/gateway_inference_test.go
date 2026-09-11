// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package orchestrator

import (
	_ "embed"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	agentmodel "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	kubecomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

//go:embed fixtures/gateway_inference.yaml
var gatewayInferenceManifest string

func deployGatewayInferenceTestResources(e config.Env, kubeProvider *kubernetes.Provider) (*kubecomp.Workload, error) {
	workload := &kubecomp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "gateway-inference-test-resources", workload); err != nil {
		return nil, err
	}

	_, err := yaml.NewConfigGroup(e.Ctx(), "gateway-inference-test-resources", &yaml.ConfigGroupArgs{
		YAML: []string{gatewayInferenceManifest},
	}, pulumi.Provider(kubeProvider), pulumi.Parent(workload))
	if err != nil {
		return nil, err
	}

	return workload, nil
}

func (suite *k8sSuite) TestGatewayAPICRManifest() {
	expectAtLeastOneManifest{
		test: func(payload *aggregator.OrchestratorManifestPayload, manif manifest) bool {
			return payload.Type == agentmodel.TypeCollectorManifestCR &&
				manif.APIVersion == "gateway.networking.k8s.io/v1" &&
				manif.Kind == "HTTPRoute" &&
				manif.Metadata.Name == "gateway-api-e2e"
		},
		message: "find a Gateway API HTTPRoute manifest collected through the built-in configuration",
		timeout: defaultTimeout,
	}.Assert(suite)
}

func (suite *k8sSuite) TestGatewayInferenceCRManifest() {
	expectAtLeastOneManifest{
		test: func(payload *aggregator.OrchestratorManifestPayload, manif manifest) bool {
			return payload.Type == agentmodel.TypeCollectorManifestCR &&
				manif.APIVersion == "inference.networking.k8s.io/v1" &&
				manif.Kind == "InferencePool" &&
				manif.Metadata.Name == "inference-pool-e2e"
		},
		message: "find an Inference Extension InferencePool manifest collected through the built-in configuration",
		timeout: defaultTimeout,
	}.Assert(suite)
}
