package examples

import (
	_ "embed"
	"os"
	"strconv"
	"testing"
	"time"

	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/local"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/stretchr/testify/assert"
)

type localLogsTestSuite struct {
	e2e.BaseSuite[environments.DockerLocal]
}

func TestE2ELocalFakeintakeSuite(t *testing.T) {
	devModeEnv, _ := os.LookupEnv("E2E_DEVMODE")
	options := []e2e.SuiteOption{
		e2e.WithProvisioner(
			local.Provisioner(
				local.WithAgentOptions(
					agentparams.WithLatest(),
					agentparams.WithIntegration("custom_logs.d", customLogsConfig),
					agentparams.WithLogs(),
					// Setting hostname to test name due to fact Agent can't
					// work out it's hostname in a container correctly
					agentparams.WithHostname(t.Name())))),
	}
	if devMode, err := strconv.ParseBool(devModeEnv); err == nil && devMode {
		options = append(options, e2e.WithDevMode())
	}
	e2e.Run(t, &localFakeintakeSuiteMetrics{}, options...)
}

func (s *localLogsTestSuite) TestLogs() {
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
