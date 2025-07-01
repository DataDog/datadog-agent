// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent_onboarding

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-onboarding/common"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/agent-onboarding/provisioners"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	"strings"
	"testing"
)

type localKindSuite struct {
	k8sSuite
}

func (s *localKindSuite) SetupSuite() {
	// use `s.local = true` to use to local kind provisioner
	s.local = true
	s.BaseSuite.SetupSuite()
}

func TestLocalKindSuite(t *testing.T) {
	operatorOptions := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		operatorparams.WithHelmValues("installCRDs: false"),
	}

	provisionerOptions := []provisioners.KubernetesProvisionerOption{
		provisioners.WithTestName("e2e-operator"),
		provisioners.WithOperatorOptions(operatorOptions...),
		provisioners.WithoutDDA(),
		provisioners.WithLocal(true),
	}

	e2eOpts := []e2e.SuiteOption{
		e2e.WithStackName(fmt.Sprintf("operator-localkind-%s", strings.ReplaceAll(common.K8sVersion, ".", "-"))),
		e2e.WithProvisioner(provisioners.KubernetesProvisioner(provisionerOptions...)),
	}

	e2e.Run(t, &localKindSuite{}, e2eOpts...)
}
