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

//go:embed fixtures/kubeai.yaml
var kubeAIManifest string

func deployKubeAITestResource(e config.Env, kubeProvider *kubernetes.Provider) (*kubecomp.Workload, error) {
	workload := &kubecomp.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "kubeai-test-resource", workload); err != nil {
		return nil, err
	}

	_, err := yaml.NewConfigGroup(e.Ctx(), "kubeai-test-resource", &yaml.ConfigGroupArgs{
		YAML: []string{kubeAIManifest},
	}, pulumi.Provider(kubeProvider), pulumi.Parent(workload))
	if err != nil {
		return nil, err
	}

	return workload, nil
}

func (suite *k8sSuite) TestKubeAICRManif() {
	expectAtLeastOneManifest{
		test: func(payload *aggregator.OrchestratorManifestPayload, manif manifest) bool {
			return payload.Type == agentmodel.TypeCollectorManifestCR &&
				manif.APIVersion == "kubeai.org/v1" &&
				manif.Kind == "Model" &&
				manif.Metadata.Name == "kubeai-e2e"
		},
		message: "find a KubeAI Model manifest collected through the built-in v1 collector",
		timeout: defaultTimeout,
	}.Assert(suite)
}
