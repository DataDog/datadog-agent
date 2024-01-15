// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	_ "embed"
	"testing"
	"time"

	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/stretchr/testify/assert"
)

type vmFakeintakeSuite struct {
	e2e.BaseSuite[environments.Host]
}

//go:embed testfixtures/custom_logs.yaml
var customLogsConfig string

func TestE2EVMFakeintakeSuite(t *testing.T) {
	e2e.Run(t, &vmFakeintakeSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithAgentOptions(
				agentparams.WithIntegration("custom_logs.d", customLogsConfig),
				agentparams.WithLogs(),
			),
		),
	))
}

func (s *vmFakeintakeSuite) TestLogs() {
	fakeintake := s.Env().FakeIntake.Client()
	// part 1: no logs
	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := fakeintake.FilterLogs("custom_logs")
		assert.NoError(c, err)
		assert.Equal(c, len(logs), 0, "logs received while none expected")
	}, 5*time.Minute, 10*time.Second)
	s.EventuallyWithT(func(c *assert.CollectT) {
		// part 2: generate logs
		s.Env().RemoteHost.MustExecute("echo 'totoro' >> /tmp/test.log")
		// part 3: there should be logs
		names, err := fakeintake.GetLogServiceNames()
		assert.NoError(c, err)
		assert.Greater(c, len(names), 0, "no logs received")
		logs, err := fakeintake.FilterLogs("custom_logs")
		assert.NoError(c, err)
		assert.Equal(c, len(logs), 1, "expecting 1 log from 'custom_logs'")
		logs, err = fakeintake.FilterLogs("custom_logs", fi.WithMessageContaining("totoro"))
		assert.NoError(c, err)
		assert.Equal(c, len(logs), 1, "expecting 1 log from 'custom_logs' with 'totoro' content")
	}, 5*time.Minute, 10*time.Second)
}
