// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testinfradefinition

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/docker"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dockerSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestDocker(t *testing.T) {
	isCI, err := strconv.ParseBool(os.Getenv("CI"))
	require.NoError(t, err)
	if isCI {
		t.Skipf("blocked by APL-2786")
	}
	e2e.Run(t, &dockerSuite{}, e2e.WithProvisioner(awsdocker.Provisioner()))
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
