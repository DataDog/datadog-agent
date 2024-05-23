// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	"os"
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	dclocal "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/local"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/stretchr/testify/assert"
)

type myLocalPlusDockerSuite struct {
	e2e.BaseSuite[environments.DockerLocal]
}

func TestLighttpdOnDockerFromLocalHost(t *testing.T) {
	suiteParams := []e2e.SuiteOption{
		e2e.WithProvisioner(
			dclocal.Provisioner(
				dclocal.WithoutFakeIntake(),
				dclocal.WithDocker(),
				dclocal.WithAgentOptions(
					agentparams.WithLatest(),
					// Setting hostname to test name due to fact Agent can't
					// work out it's hostname in a container correctly
					agentparams.WithHostname(t.Name())))),
	}

	if isDevModeEnabled {
		suiteParams = append(suiteParams, e2e.WithDevMode())
	}

	e2e.Run(t, &myLocalPlusDockerSuite{}, suiteParams...)
}

func (v *myLocalPlusDockerSuite) TestDockerWorks() {
	res := v.Env().RemoteHost.MustExecute("docker run hello-world")
	assert.Contains(v.T(), res, "Hello from Docker!")
}

var isDevModeEnabled = getDevModeEnv()

func getDevModeEnv() bool {
	denv, _ := os.LookupEnv("E2E_DEVMODE")
	if dm, err := strconv.ParseBool(denv); err == nil && dm {
		return true
	}
	return false
}
