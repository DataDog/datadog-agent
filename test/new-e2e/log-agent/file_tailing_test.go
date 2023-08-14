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
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/e2e/params"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2params"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2vm"
	"github.com/cenkalti/backoff/v4"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
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
	return e2e.EnvFactoryStackDef(
		// This function creates the environment, including the virtual machine and agent installer
		func(ctx *pulumi.Context) (*e2e.FakeIntakeEnv, error) {
			// Create a new EC2 virtual machine, default Ubuntu OS ( WithOS(os.UbuntuOS, os.AMD64Arch)
			vm, err := ec2vm.NewEc2VM(ctx, vmParams...)
			if err != nil {
				return nil, err
			}

			// Create a new fake intake exporter.
			fakeintakeExporter, err := aws.NewEcsFakeintake(vm.GetAwsEnvironment())
			if err != nil {
				return nil, err
			}

			// Define agent parameters, including integration with custom_logs logs.
			agentParams = append(agentParams, agentparams.WithFakeintake(fakeintakeExporter))
			agentParams = append(agentParams, agentparams.WithLogs())
			agentParams = append(agentParams, agentparams.WithIntegration("custom_logs.d", config))

			// Create a new agent installer.
			installer, err := agent.NewInstaller(vm, agentParams...)
			if err != nil {
				return nil, err
			}

			// Return the environment setup.
			return &e2e.FakeIntakeEnv{
				VM:         client.NewVM(vm),
				Agent:      client.NewAgent(installer),
				Fakeintake: client.NewFakeintake(fakeintakeExporter),
			}, nil
		},
	)
}

// TestE2EVMFakeintakeSuite runs the end-to-end test suite for the log agent with virtual machine and fake intake.
func TestE2EVMFakeintakeSuite(t *testing.T) {
	e2e.Run(t, &vmFakeintakeSuite{}, logsExampleStackDef(nil), params.WithDevMode())
}

// TestLogs executes a three-part test of the log agent's interaction with the fake intake system.
func (s *vmFakeintakeSuite) TestLogs() {
	t := s.T()
	fakeintake := s.Env().Fakeintake

	// part 1: no logs
	// This part verifies that no logs are received initially.
	err := backoff.Retry(

		func() error {
			s.Env().VM.ExecuteWithError("sudo rm -f /var/log/hello-world.log")
			s.Env().VM.ExecuteWithError("sudo rm -f /var/log/hello-world.log.old")
			s.Env().VM.ExecuteWithError("sudo touch /var/log/hello-world.log")

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
		}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 120))
	require.NoError(t, err)

	// part 2: generate logs
	// This part generates a test log and verifies that it was created successfully.

	var str string
	for i := 0; i < 100; i++ {
		str += "hello-world"
	}

	s.Env().VM.Execute("sudo chmod 777 /var/log/hello-world.log")
	generateLog := fmt.Sprintf("echo %s > /var/log/hello-world.log", str)
	_, err = s.Env().VM.ExecuteWithError(generateLog)
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

		// Part 4: Block permission and check the Agent status
		_, err = s.Env().VM.ExecuteWithError("sudo chmod 000 /var/log/hello-world.log")
		require.NoError(t, err)

		// Sleep 10s to make sure agent status for the log file is updated
		time.Sleep(10 * time.Second)

		// Check the Agent status
		statusOutput, err := s.Env().VM.ExecuteWithError("datadog-agent status | grep -A 10 'custom_logs'")
		if err != nil || !strings.Contains(statusOutput, "Status: OK") {
			if strings.Contains(statusOutput, "permission denied") {
				t.Log("Log file is inaccessible due to permission issues.")
			} else {
				t.Log("Log file is inaccessible.")
			}
		} else {
			t.Error("Expected the log file to be inaccessible, but agent status says log status is OK.")
		}

		// Part 5: Restore permissions
		_, err = s.Env().VM.ExecuteWithError("sudo chmod 777 /var/log/hello-world.log")
		require.NoError(t, err)

		// Part 6: Stop the agent, generate new logs, start the agent
		_, err = s.Env().VM.ExecuteWithError("sudo service datadog-agent stop")
		require.NoError(t, err)
		time.Sleep(5 * time.Second)

		_, err = s.Env().VM.ExecuteWithError(generateLog) // Appending logs this time.
		require.NoError(t, err)

		_, err = s.Env().VM.ExecuteWithError("sudo service datadog-agent start")
		require.NoError(t, err)

		// Part 7: Rotate the log file and check if the agent is tailing the new log file.

		// Rotate the log file
		_, err = s.Env().VM.ExecuteWithError("sudo mv /var/log/hello-world.log /var/log/hello-world.log.old && sudo touch /var/log/hello-world.log")
		require.NoError(t, err)

		// Verify the old log file's existence after rotation
		_, err = s.Env().VM.ExecuteWithError("ls /var/log/hello-world.log.old")
		require.NoError(t, err, "Old log file missing after rotation")

		// Grant new log file permission
		_, err = s.Env().VM.ExecuteWithError("sudo chmod 777 /var/log/hello-world.log")
		require.NoError(t, err)

		// Check if agent is tailing new log file via agent status
		time.Sleep(10 * time.Second) // Making sure agent has time to start
		newStatusOutput, err := s.Env().VM.ExecuteWithError("datadog-agent status | grep -A 10 'custom_logs'")
		require.NoError(t, err)
		if !strings.Contains(newStatusOutput, "Path: /var/log/hello-world.log") {
			t.Error("The agent is not tailing the expected log file.")
		}

		// Generate new log
		newLogContent := "hello-world-new-content"
		generateNewLog := fmt.Sprintf("echo %s > /var/log/hello-world.log", newLogContent)
		_, err = s.Env().VM.ExecuteWithError(generateNewLog)
		require.NoError(t, err)

		// Verify log file's content
		logs, err = fakeintake.FilterLogs("hello", fi.WithMessageContaining(newLogContent))
		if err != nil {
			return err
		}
		if len(logs) != 1 {
			return fmt.Errorf("received %v logs with 'hello-world-new-content', expecting 1", len(logs))
		}
		require.NoError(t, err)

		return nil
	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 60))

	require.NoError(t, err)
}
