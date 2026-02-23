// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type agentSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestAgentSuite(t *testing.T) {
	e2e.Run(t, &agentSuite{}, e2e.WithProvisioner(awshost.ProvisionerNoFakeIntake()), e2e.WithSkipCoverage())
}

func (v *agentSuite) TestAgentCommandNoArg() {
	status, err := v.Env().Agent.Client.StatusWithError()
	require.NoError(v.T(), err)
	assert.NotNil(v.T(), status)
	assert.NotEmpty(v.T(), status.Content)
}
