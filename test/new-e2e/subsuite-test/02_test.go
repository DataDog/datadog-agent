package subSuite

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2os"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/stretchr/testify/require"
)

func (s *vmFakeintakeSuite) Test2WindowsTests() {
	windowsConfig :=
		`logs:
  - type: file
    path: 'C:\\logs\\my-log.log'
    service: hello-windows
    source: custom_windows_log
`
	s.UpdateEnv(e2e.FakeIntakeStackDef([]ec2params.Option{ec2params.WithOS(ec2os.WindowsOS)}, agentparams.WithLogs(), agentparams.WithIntegration("custom_logs.d", windowsConfig)))

	time.Sleep(1 * time.Second)
	s.T().Run("WindowsSubTest1", func(t *testing.T) {
		s.T().Log("Running WindowsSubTest1")
		s.WindowsSubTest1()
	})
	time.Sleep(1 * time.Second)
	s.T().Run("WindowsSubTest2", func(t *testing.T) {
		s.T().Log("Running WindowsSubTest2")
		s.WindowsSubTest2()
	})

}

func (s *vmFakeintakeSuite) WindowsSubTest1() {
	t := s.T()

	// Create a log file in Windows
	logPath := "C:\\logs\\my-log.log"
	createCmd := fmt.Sprintf("New-Item -Path %s -ItemType file -Force", logPath)

	out, err := s.Env().VM.ExecuteWithError(createCmd)
	s.T().Log(out)
	require.NoErrorf(t, err, "Failed to create log file")
	time.Sleep(10 * time.Second)

	// Check if the log file exists
	checkCmd := fmt.Sprintf("Test-Path %s", logPath)
	s.T().Log(checkCmd)
	output, err := s.Env().VM.ExecuteWithError(checkCmd)
	s.T().Log(output)
	require.NoError(t, err, "Failed to check log file existence")
	require.Contains(t, output, "True", "Log file does not exist when it should")
}

func (s *vmFakeintakeSuite) WindowsSubTest2() {
	t := s.T()

	// Remove the log file in Windows
	logPath := "C:\\logs\\my-log.log"
	removeCmd := fmt.Sprintf("Remove-Item -Path %s -Force", logPath)
	s.T().Log(removeCmd)
	out, err := s.Env().VM.ExecuteWithError(removeCmd)
	require.NoErrorf(t, err, "Failed to remove log file")
	s.T().Log(out)

	time.Sleep(10 * time.Second)
	// Check if the log file has been removed
	checkCmd := fmt.Sprintf("Test-Path %s", logPath)
	s.T().Log(checkCmd)
	output, err := s.Env().VM.ExecuteWithError(checkCmd)
	s.T().Log(output)
	require.NoError(t, err, "Failed to check log file existence")
	require.Contains(t, output, "False", "Log file still exists when it should not")
}
