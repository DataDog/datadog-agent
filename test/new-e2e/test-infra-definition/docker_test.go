// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

type dockerSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestDocker(t *testing.T) {
	var fakeintakeOpts []fakeintake.Option

	// When we modify the fakeintake, this test will run with the new version of the fakeintake
	if fakeintakeImage, ok := os.LookupEnv("FAKEINTAKE_IMAGE_OVERRIDE"); ok {
		t.Logf("Running with fakeintake image %s", fakeintakeImage)
		fakeintakeOpts = append(fakeintakeOpts, fakeintake.WithImageURL(fakeintakeImage))
	}
	e2e.Run(t, &dockerSuite{}, e2e.WithProvisioner(awsdocker.Provisioner(awsdocker.WithRunOptions(ec2docker.WithFakeIntakeOptions(fakeintakeOpts...)))), e2e.WithSkipCoverage())
}

func (v *dockerSuite) TestExecuteCommand() {
	agentVersion := v.Env().Agent.Client.Version()
	regexpVersion := regexp.MustCompile(`.*Agent .* - Commit: .* - Serialization version: .* - Go version: .*`)

	v.Require().Truef(regexpVersion.MatchString(agentVersion), fmt.Sprintf("%v doesn't match %v", agentVersion, regexpVersion))

	// args is used to test client.WithArgs. The values of the arguments are not relevant.
	args := agentclient.WithArgs([]string{"-n", "-c", "."})
	version := v.Env().Agent.Client.Version(args)
	v.Require().Truef(regexpVersion.MatchString(version), fmt.Sprintf("%v doesn't match %v", version, regexpVersion))

	v.EventuallyWithT(func(tt *assert.CollectT) {
		metrics, err := v.Env().FakeIntake.Client().GetMetricNames()
		assert.NoError(tt, err)
		assert.Contains(tt, metrics, "system.uptime", fmt.Sprintf("metrics %v doesn't contain system.uptime", metrics))
	}, 2*time.Minute, 10*time.Second)
}
