package logAgent

import (
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"testing"

	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/require"
)

// vmFakeintakeSuite defines a test suite for the log agent interacting with a virtual machine and fake intake.
type vmFakeintakeSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

// logsExampleStackDef returns the stack definition required for the log agent test suite.
func logsExampleStackDef(vmParams []ec2params.Option, agentParams ...agentparams.Option) *e2e.StackDefinition[e2e.FakeIntakeEnv] {
	config :=
		`logs:
  - type: file
    path: '/var/log/hello-world.log'
    service: hello
    source: custom_log
`
	return e2e.FakeIntakeStackDef(nil, agentparams.WithLogs(), agentparams.WithIntegration("custom_logs.d", config))

}

// TestE2EVMFakeintakeSuite runs the end-to-end test suite for the log agent with virtual machine and fake intake.
func TestE2EVMFakeintakeSuite(t *testing.T) {
	e2e.Run(t, &vmFakeintakeSuite{}, logsExampleStackDef(nil), params.WithDevMode())
}

func (s *vmFakeintakeSuite) TestLogCollection() {
	t := s.T()
	fakeintake := s.Env().Fakeintake
	fakeintake.FlushServerAndResetAggregators()
	s.cleanUp()
	s.Env().VM.Execute("sudo touch /var/log/hello-world.log")

	// part 1: check for no logs
	// This part verifies that no logs are received initially.
	err := backoff.Retry(

		func() error {
			logs, err := fakeintake.FilterLogs("hello")
			if err != nil {
				return err
			}
			if len(logs) != 0 {
				cat := s.Env().VM.Execute("cat /var/log/hello-world.log")
				lm := fmt.Sprintf("%v logs %v received while none expected", len(logs), cat)
				return errors.New(lm)
			}
			return nil
		}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 100))
	require.NoError(t, err)

	// part 2: generate logs
	// This part generates a test log and verifies that it was created successfully.

	s.Env().VM.Execute("sudo chmod 777 /var/log/hello-world.log")
	_, err = s.Env().VM.ExecuteWithError(generateLog("hello-world"))
	require.NoError(t, err)

	// part 3: there should be logs
	// This part verifies that the test log was received and contains the expected content.
	err = backoff.Retry(func() error {
		names, err := fakeintake.GetLogServiceNames()
		// Checks for received logs and validate content.
		if err != nil {
			return err
		}

		// Check to see if there are any logs base on the intake service name
		if len(names) == 0 {
			return errors.New("no logs found in intake service")
		}

		// Check to see if service name matches
		service := "hello"
		logs, err := fakeintake.FilterLogs(service)
		if err != nil {
			return err
		}

		if len(logs) != 1 {
			m := fmt.Sprintf("no logs with service matching '%s' found, instead got '%s'", service, names)
			return errors.New(m)
		}

		// check if log's contain matches what is produced
		logs, err = fakeintake.FilterLogs("hello", fi.WithMessageContaining("hello-world"))
		if err != nil {
			return err
		}
		if len(logs) != 1 {
			return fmt.Errorf("received %v logs with 'hello-world', expecting 1", len(logs))
		}

		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 120))
	require.NoError(t, err)
}

func (s *vmFakeintakeSuite) TestLogPermission() {
	t := s.T()

	// Part 4: Block permission and check the Agent status
	s.Env().VM.Execute("sudo chmod 000 /var/log/hello-world.log")
	// require.NoError(t, err)

	erro := backoff.Retry(func() error {
		// Check the Agent status

		statusOutput := s.Env().VM.Execute("sudo datadog-agent status | grep -A 10 'custom_logs'")

		// Check if the status indicates that the log file is accessible
		if strings.Contains(statusOutput, "Status: OK") {
			t.Logf("Log file is unexpectedly accessible.")
			return errors.New("log file is unexpectedly accessible")
		} else if strings.Contains(statusOutput, "permission denied") {
			// This is the expected behavior when the log file is inaccessible
			t.Logf("Log file correctly inaccessible.")
		} else {
			// If the status is neither "OK" nor "permission denied", log the unexpected status for debugging
			t.Logf("Unexpected agent status: \n%s", statusOutput)
			return errors.New("unexpected agent status")
		}

		// Part 5: Restore permissions
		s.Env().VM.Execute("sudo chmod 777 /var/log/hello-world.log")

		// Part 6: Stop the agent, generate new logs, start the agent
		s.Env().VM.Execute("sudo service datadog-agent stop")

		s.Env().VM.Execute(generateLog("hello-world")) // Appending logs this time.

		s.Env().VM.Execute("sudo service datadog-agent start")

		// Check the Agent status
		statusOutput = s.Env().VM.Execute("sudo datadog-agent status | grep -A 10 'custom_logs'")
		if strings.Contains(statusOutput, "Status: OK") {
			if strings.Contains(statusOutput, "permission denied") {
				t.Errorf("Expecting log file to be accessible, received: %s", statusOutput)
			}
		} else {
			t.Log("Agent is collecting logs as expected")
		}
		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Microsecond), 120))
	require.NoError(t, erro)
}

func (s *vmFakeintakeSuite) TestLogRotation() {
	t := s.T()
	fakeintake := s.Env().Fakeintake

	// Part 7: Rotate the log file and check if the agent is tailing the new log file.

	// Rotate the log file
	_, err := s.Env().VM.ExecuteWithError("sudo mv /var/log/hello-world.log /var/log/hello-world.log.old && sudo touch /var/log/hello-world.log")
	require.NoError(t, err)

	// Verify the old log file's existence after rotation
	_, err = s.Env().VM.ExecuteWithError("ls /var/log/hello-world.log.old")
	require.NoError(t, err, "Old log file missing after rotation")

	// Grant new log file permission
	_, err = s.Env().VM.ExecuteWithError("sudo chmod 777 /var/log/hello-world.log")
	require.NoError(t, err)

	// Check if agent is tailing new log file via agent status
	time.Sleep(3000 * time.Millisecond)
	newStatusOutput, err := s.Env().VM.ExecuteWithError("sudo datadog-agent status | grep -A 10 'custom_logs'")
	require.NoError(t, err)
	if !strings.Contains(newStatusOutput, "Path: /var/log/hello-world.log") {
		t.Error("The agent is not tailing the expected log file.")
	}

	// Verify log file's content
	s.Env().VM.Execute(generateLog("hello-world-new-content"))
	time.Sleep(10000 * time.Millisecond)

	logs, err := fakeintake.FilterLogs("hello", fi.WithMessageContaining("hello-world-new-content"))
	require.NoError(t, err)
	require.NotEqual(t, 0, len(logs), "received %v logs with content: %s, expecting 1 ", len(logs), logs)

	//clean up
	s.cleanUp()
}

func generateLog(content string) string {
	return fmt.Sprintf("echo %s > /var/log/hello-world.log", strings.Repeat(content, 10))
}

func (s *vmFakeintakeSuite) cleanUp() {
	s.Env().VM.Execute("sudo rm -f /var/log/hello-world.log")
	s.Env().VM.Execute("sudo rm -f /var/log/hello-world.log.old")
	time.Sleep(3000 * time.Millisecond)
	s.T().Logf("Cleaning up log files")
}
