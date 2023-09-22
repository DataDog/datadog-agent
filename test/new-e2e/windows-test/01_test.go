package subSuite

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type vmFakeintakeSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

func logsExampleStackDef(vmParams []ec2params.Option, agentParams ...agentparams.Option) *e2e.StackDefinition[e2e.FakeIntakeEnv] {
	windowsConfig :=
		`logs:
  - type: file
    path: 'C:\\logs\\hello-world.log'
    service: hello
    source: custom_log
`

	return e2e.FakeIntakeStackDef([]ec2params.Option{ec2params.WithOS(ec2os.WindowsOS)}, agentparams.WithLogs(), agentparams.WithIntegration("custom_logs.d", windowsConfig))

}
func TestE2EVMFakeintakeSuite(t *testing.T) {
	e2e.Run(t, &vmFakeintakeSuite{}, logsExampleStackDef(nil), params.WithDevMode())
}

func (s *vmFakeintakeSuite) TestWindows() {
	s.cleanUp()
	// time.Sleep(10 * time.Second)

	s.T().Run("WindowsSubTest1", func(t *testing.T) {
		s.T().Log("Running WindowsSubTest1")
		s.WindowsSubTest1()
	})
	s.T().Run("WindowsLogCollection", func(t *testing.T) {
		s.T().Log("Running WindowsLogCollection")
		s.WindowsLogCollection()
	})
}
func (s *vmFakeintakeSuite) WindowsSubTest1() {
	t := s.T()

	logPath := "C:\\logs\\my-log.log"
	createLogFile := fmt.Sprintf("New-Item -Path %s -ItemType file -Force", logPath)

	s.Env().VM.Execute(createLogFile)
	checkCmd := fmt.Sprintf("Test-Path %s", logPath)
	output := s.Env().VM.Execute(checkCmd)
	require.Contains(t, output, "True", "Log file does not exist when it should")
}

func (s *vmFakeintakeSuite) WindowsLogCollection() {
	t := s.T()
	fakeintake := s.Env().Fakeintake

	fakeintake.FlushServerAndResetAggregators()
	_, err := s.Env().VM.ExecuteWithError("New-Item -Path C:\\logs -ItemType Directory -Force")
	require.NoErrorf(t, err, "Failed to create new directory")

	_, err = s.Env().VM.ExecuteWithError("New-Item -Path C:\\logs\\hello-world.log -ItemType file")
	require.NoErrorf(t, err, "Failed to create new log file")

	s.EventuallyWithT(func(c *assert.CollectT) {
		logs, err := fakeintake.FilterLogs("hello")
		require.NoErrorf(t, err, "can't filter logs by service hello")

		if len(logs) != 0 {
			cat, _ := s.Env().VM.ExecuteWithError("Get-Content C:\\logs\\hello-world.log")
			require.NoErrorf(t, err, "%v logs %v received while none expected", len(logs), cat)
		}

	}, 5*time.Minute, 10*time.Second)

	_, err = s.Env().VM.ExecuteWithError("icacls C:\\logs\\hello-world.log /grant *S-1-1-0:F")
	require.NoErrorf(t, err, "Failed to change log file permissions")

	generateLog(s, t, "hello-world")

	s.EventuallyWithT(func(c *assert.CollectT) {
		checkLogs(s, "hello", "hello-world")
	}, 5*time.Minute, 10*time.Second)
}
