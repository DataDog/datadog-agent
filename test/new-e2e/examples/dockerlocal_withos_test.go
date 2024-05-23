// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	dclocal "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/local"

	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/stretchr/testify/assert"
)

type myLocalSuite struct {
	e2e.BaseSuite[environments.DockerLocal]
}

func TestMyLocalSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{
		e2e.WithProvisioner(
			dclocal.ProvisionerNoAgentNoFakeIntake(
				dclocal.WithInstanceOptions(ec2.WithOS(os.AmazonLinux2023)))),
	}

	if isDevModeEnabled {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	e2e.Run(t, &myLocalSuite{}, suiteParams...)
}

func (v *myLocalSuite) TestIsAmazonLinux() {
	res := v.Env().RemoteHost.MustExecute("cat /etc/os-release")
	assert.Contains(v.T(), res, "Amazon Linux")
}
