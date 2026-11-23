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

//go:embed fixtures/kserve.yaml
var kserveManifest string

func deployKServeTestResource(e config.Env, kubeProvider *kubernetes.Provider) (*kubecomp.Workload, error) {
	workload := &kubecomp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "kserve-test-resource", workload); err != nil {
		return nil, err
	}

	_, err := yaml.NewConfigGroup(e.Ctx(), "kserve-test-resource", &yaml.ConfigGroupArgs{
		YAML: []string{kserveManifest},
	}, pulumi.Provider(kubeProvider), pulumi.Parent(workload))
	if err != nil {
		return nil, err
	}

	return workload, nil
}

func (suite *k8sSuite) TestKServeCRManif() {
	expectAtLeastOneManifest{
		test: func(payload *aggregator.OrchestratorManifestPayload, manif manifest) bool {
			return payload.Type == agentmodel.TypeCollectorManifestCR &&
				manif.APIVersion == "serving.kserve.io/v1alpha1" &&
				manif.Kind == "LLMInferenceService" &&
				manif.Metadata.Name == "kserve-e2e"
		},
		message: "find a KServe LLMInferenceService manifest collected through the built-in v1alpha1 fallback",
		timeout: defaultTimeout,
	}.Assert(suite)
}
