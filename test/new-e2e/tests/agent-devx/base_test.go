// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentdevx

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
)

type agentBaselineSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestAgentBaselineSuite(t *testing.T) {
	e2e.Run(t, &agentBaselineSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()))
}

func (s *agentBaselineSuite) TestAgentuns() {
	_, err := s.Env().Agent.Client.StatusWithError()
	s.NoError(err)
}
