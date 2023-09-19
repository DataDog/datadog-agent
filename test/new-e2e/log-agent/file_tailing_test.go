package logAgent

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"testing"

	fi "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/cenkalti/backoff/v4"
	"github.com/stretchr/testify/require"
)

// vmFakeintakeSuite defines a test suite for the log agent interacting with a virtual machine and fake intake.
type vmFakeintakeSuite struct {
	e2e.Suite[e2e.FakeIntakeEnv]
}

var exponentialBackOff = backoff.NewExponentialBackOff()

func init() {
	exponentialBackOff = backoff.NewExponentialBackOff()
	exponentialBackOff.MaxInterval = 30 * time.Second
	exponentialBackOff.MaxElapsedTime = 2 * time.Minute
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

// TestE2EVMFakeintakeSuite runs the E2E test suite for the log agent with a VM and fake intake.
func TestE2EVMFakeintakeSuite(t *testing.T) {
	e2e.Run(t, &vmFakeintakeSuite{}, logsExampleStackDef(nil))
}

func (s *vmFakeintakeSuite) TestLogCollection() {
	t := s.T()
	fakeintake := s.Env().Fakeintake
	fakeintake.FlushServerAndResetAggregators()
	s.Env().VM.Execute("sudo touch /var/log/hello-world.log")

	// part 1: check for no logs
	err := backoff.Retry(
		func() error {
			logs, err := fakeintake.FilterLogs("hello")
			if err != nil {
				return fmt.Errorf("can't filter logs by service hello: %v", err)
			}
			if len(logs) != 0 {
				cat, _ := s.Env().VM.ExecuteWithError("cat /var/log/hello-world.log")
				return fmt.Errorf("%v logs %v received while none expected", len(logs), cat)
			}
			return nil
		}, backoff.WithMaxRetries(exponentialBackOff, 10))
	require.NoErrorf(t, err, "Unexpected logs found")

	// part 2: generate logs
	_, err = s.Env().VM.ExecuteWithError("sudo chmod 777 /var/log/hello-world.log")
	require.NoErrorf(t, err, "Failed to change log file permissions")
	err = generateLog(s, t, "hello-world")
	require.NoErrorf(t, err, "Failed to generate logs")

	// part 3: there should be logs
	err = checkLogs(s, "hello", "hello-world")
	require.NoErrorf(t, err, "Logs not found or unexpected number of logs found")
}

func (s *vmFakeintakeSuite) TestLogPermission() {
	t := s.T()

	// Part 4: Block permission and check the Agent status
	s.Env().VM.Execute("sudo chmod 000 /var/log/hello-world.log")

	err := backoff.Retry(func() error {
		// Check the Agent status
		statusOutput, err := s.Env().VM.ExecuteWithError("sudo datadog-agent status | grep -A 10 'custom_logs'")
		if err != nil {
			return fmt.Errorf("Issue running agent status: %s", err)
		}
		// Check if the status indicates that the log file is accessible
		if strings.Contains(statusOutput, "Status: OK") {
			return errors.New("log file is unexpectedly accessible")
		} else if strings.Contains(statusOutput, "permission denied") {
			t.Logf("Log file correctly inaccessible.")
			return nil
		} else {
			// If the status is neither "OK" nor "permission denied", log the unexpected status for debugging
			t.Logf("Unexpected agent status: \n%s", statusOutput)
			return errors.New("unexpected agent status")
		}
	}, backoff.WithMaxRetries(exponentialBackOff, 10))
	require.NoErrorf(t, err, "Failed to retrieve agent status. Ensure the agent is running")

	// Part 5: Restore permissions
	s.Env().VM.Execute("sudo chmod 777 /var/log/hello-world.log")

	// Part 6: Stop the agent, generate new logs, start the agent
	s.Env().VM.Execute("sudo service datadog-agent restart")

	err2 := generateLog(s, s.T(), "hello-world")
	require.NoErrorf(t, err2, "Failed to generate logs") // Appending logs this time.

	// Check the Agent status
	err3 := backoff.Retry(func() error {
		statusOutput, err := s.Env().VM.ExecuteWithError("sudo datadog-agent status | grep -A 10 'custom_logs'")
		if err != nil {
			return fmt.Errorf("Issue running agent status: %s", err)
		}
		if strings.Contains(statusOutput, "Status: OK") {
			t.Log("Agent is collecting logs with expected permission")
			return nil
		} else {
			return fmt.Errorf("Expecting log file to be accessible but it is inaccessible instead")
		}
	}, backoff.WithMaxRetries(exponentialBackOff, 10))
	require.NoErrorf(t, err3, "Failed to retrieve agent status. Ensure the agent is running")
}

func (s *vmFakeintakeSuite) TestLogRotation() {
	t := s.T()
	// Clean up once test is finished running
	defer s.cleanUp()

	// Part 7: Rotate the log file and check if the agent is tailing the new log file.
	// Rotate the log file
	s.Env().VM.Execute("sudo mv /var/log/hello-world.log /var/log/hello-world.log.old && sudo touch /var/log/hello-world.log")

	// Verify the old log file's existence after rotation
	s.Env().VM.Execute("ls /var/log/hello-world.log.old")

	// Grant new log file permission
	s.Env().VM.Execute("sudo chmod 777 /var/log/hello-world.log")

	// Check if agent is tailing new log file via agent status
	err := backoff.Retry(
		func() error {
			newStatusOutput, err := s.Env().VM.ExecuteWithError("sudo datadog-agent status | grep -A 10 'custom_logs'")
			if err != nil {
				return fmt.Errorf("Issue running agent status: %s", err)
			}
			if !strings.Contains(newStatusOutput, "Path: /var/log/hello-world.log") {
				return fmt.Errorf("The agent is not tailing the expected log file, instead: \n %s", newStatusOutput)
			}

			return nil
		}, backoff.WithMaxRetries(exponentialBackOff, 10))
	require.NoErrorf(t, err, "Failed to retrieve agent status. Ensure the agent is running")

	// Generate new log
	err2 := generateLog(s, s.T(), "hello-world-new-content")
	require.NoErrorf(t, err2, "Failed to generate logs")

	// Verify Log's content is generated and submitted
	err3 := checkLogs(s, "hello", "hello-world-new-content")
	require.NoErrorf(t, err3, "Logs not found or unexpected number of logs found")

}

// generateLog generates and verify log contents
func generateLog(s *vmFakeintakeSuite, t *testing.T, content string) error {
	t.Log("Generating Log")
	s.Env().VM.Execute(fmt.Sprintf("echo %s > /var/log/hello-world.log", strings.Repeat(content, 10)))

	// This part check to see if log has been generated
	err := backoff.Retry(func() error {
		output := s.Env().VM.Execute("cat /var/log/hello-world.log")
		if strings.Contains(output, content) {
			t.Log("Log is generated")
			return nil // Log has been generated
		}
		return errors.New("log not yet generated")
	}, backoff.WithMaxRetries(exponentialBackOff, 10))

	return err
}

// cleanUp cleans up any existing log files
func (s *vmFakeintakeSuite) cleanUp() {
	s.Env().VM.Execute("sudo rm -f /var/log/hello-world.log")
	s.Env().VM.Execute("sudo rm -f /var/log/hello-world.log.old")

	err := backoff.Retry(func() error {
		output, err := s.Env().VM.ExecuteWithError("ls /var/log/hello-world.log /var/log/hello-world.log.old")
		if err != nil {
			return errors.New("Having issue cleaning log files, retrying...")
		} else {
			s.T().Logf("Finished cleaning up %s", output)
		}
		return nil
	}, backoff.WithMaxRetries(exponentialBackOff, 10))
	if err != nil {
		s.T().Logf("Failed to clean up log files after retries: %v \n", err)
	} else {
		s.T().Logf("Cleaning up log files")
	}
}

// checkLogs checks and verifies logs inside the intake
func checkLogs(fakeintake *vmFakeintakeSuite, service, content string) error {
	client := fakeintake.Env().Fakeintake

	return backoff.Retry(func() error {
		names, err := client.GetLogServiceNames()
		if err != nil {
			return fmt.Errorf("found error %s", err)
		}
		if len(names) == 0 {
			return errors.New("no logs found in intake service")
		}
		logs, err := client.FilterLogs(service)
		if err != nil {
			return fmt.Errorf("found error %s", err)
		}
		if len(logs) < 1 {
			return fmt.Errorf("no logs with service matching '%s' found, instead got '%s'", service, names)
		}
		logs, err = client.FilterLogs(service, fi.WithMessageContaining(content))
		if err != nil {
			return fmt.Errorf("found error %s", err)
		}
		if len(logs) != 1 {
			return fmt.Errorf("received %v logs with '%s', expecting 1", len(logs), content)
		}
		return nil
	}, backoff.WithMaxRetries(exponentialBackOff, 120))
}
