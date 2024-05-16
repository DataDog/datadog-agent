// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package examples

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	dclocal "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/local"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
)

type tmpSuite struct {
	e2e.BaseSuite[environments.DockerLocal]
}

func TestSimpleLocalAgentRun(t *testing.T) {
	devModeEnv, _ := os.LookupEnv("E2E_DEVMODE")
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(
			dclocal.Provisioner(
				dclocal.WithoutFakeIntake(),
				dclocal.WithAgentOptions(
					agentparams.WithLatest(),
					// Setting hostname to test name due to fact Agent can't
					// work out it's hostname in a container correctly
					agentparams.WithHostname(t.Name())))),
	}

	if devMode, err := strconv.ParseBool(devModeEnv); err == nil && devMode {
		options = append(options, e2e.WithDevMode())
	}
	e2e.Run(t, &tmpSuite{}, options...)
}

func (d *tmpSuite) TestExecute() {
	d.T().Log("Running test")
	vm := d.Env().RemoteHost

	out, err := vm.Execute("whoami")
	d.Require().NoError(err)
	d.Require().NotEmpty(out)
}

func (d *tmpSuite) TestAgentCommand() {
	agentVersion := d.Env().Agent.Client.Version()
	regexpVersion := regexp.MustCompile(`.*Agent .* - Commit: .* - Serialization version: .* - Go version: .*`)

	d.Require().Truef(regexpVersion.MatchString(agentVersion), fmt.Sprintf("%v doesn't match %v", agentVersion, regexpVersion))
	// args is used to test client.WithArgs. The values of the arguments are not relevant.
	args := agentclient.WithArgs([]string{"-n", "-c", "."})
	version := d.Env().Agent.Client.Version(args)

	d.Require().Truef(regexpVersion.MatchString(version), fmt.Sprintf("%v doesn't match %v", version, regexpVersion))
}
