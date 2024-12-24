// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent_onboarding

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-onboarding/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-onboarding/provisioners"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	"testing"
)

type localKindSuite struct {
	k8sSuite
}

func (s *localKindSuite) SetupSuite() {
	s.local = true
}

func TestLocalKindSuite(t *testing.T) {
	operatorOptions := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		operatorparams.WithHelmValues("installCRDs: false"),
	}

	provisionerOptions := []provisioners.KubernetesProvisionerOption{
		provisioners.WithK8sVersion(common.K8sVersion),
		provisioners.WithOperatorOptions(operatorOptions...),
		provisioners.WithoutDDA(),
		provisioners.WithLocal(true),
	}

	e2e.Run(t, &localKindSuite{}, e2e.WithProvisioner(provisioners.KubernetesProvisioner(provisionerOptions...)))
}
