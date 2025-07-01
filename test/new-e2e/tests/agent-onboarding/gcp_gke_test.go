// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent_onboarding

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-onboarding/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-onboarding/provisioners"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	"testing"
)

type gkeSuite struct {
	k8sSuite
}

func TestGKESuite(t *testing.T) {
	operatorOptions := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithHelmValues(`
installCRDs: false`),
	}

	ddaOpts := []agentwithoperatorparams.Option{
		agentwithoperatorparams.WithNamespace(common.NamespaceName),
		agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{Name: "dda", YamlConfig: `
spec:
  override:
    nodeAgent:
      image:
        tag: "latest"
`}),
	}
	provisionerOptions := []provisioners.KubernetesProvisionerOption{
		provisioners.WithTestName("e2e-operator"),
		provisioners.WithK8sVersion(common.K8sVersion),
		provisioners.WithOperatorOptions(operatorOptions...),
		provisioners.WithDDAOptions(ddaOpts...),
		provisioners.WithoutDDA(),
	}

	e2eOpts := []e2e.SuiteOption{
		e2e.WithProvisioner(provisioners.KubernetesProvisioner(provisionerOptions...)),
		e2e.WithDevMode(),
		e2e.WithSkipDeleteOnFailure(),
	}

	e2e.Run(t, &gkeSuite{}, e2eOpts...)
}
