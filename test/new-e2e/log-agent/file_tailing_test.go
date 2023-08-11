package logAgent

import (
	_ "embed"
	"errors"
	"fmt"
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
    service: hello-world
    source: custom_log
`
	return e2e.EnvFactoryStackDef(
		// This function creates the environment, including the virtual machine and agent installer
		func(ctx *pulumi.Context) (*e2e.FakeIntakeEnv, error) {
			// Create a new EC2 virtual machine.
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
			fmt.Printf("custom_logs.d, %s", config)

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
			s.Env().VM.Execute("sudo rm -f /var/log/hello-world.log")
			s.Env().VM.Execute("sudo touch /var/log/hello-world.log")
			s.Env().VM.Execute("sudo chmod 777 /var/log/hello-world.log")

			logs, err := fakeintake.FilterLogs("hello-world\n")

			if err != nil {
				return err
			}
			if len(logs) != 0 {
				fmt.Printf("%v", logs)
				return errors.New("logs received while none expected")
			}
			return nil
		}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 60))
	require.NoError(t, err)

	// part 2: generate logs
	// This part generates a test log and verifies that it was created successfully.

	var str string
	for i := 0; i < 100; i++ {
		str += "hello-world"
	}

	lg := fmt.Sprintf("echo %s > /var/log/hello-world.log", str)
	_, err = s.Env().VM.ExecuteWithError(lg)
	fmt.Printf("%s", s.Env().VM.Execute("cat /var/log/hello-world.log"))
	require.NoError(t, err)

	// part 3: there should be logs
	// This part verifies that the test log was received and contains the expected content.
	err = backoff.Retry(func() error {
		names, err := fakeintake.GetLogServiceNames()
		// Checks for received logs and validate content.
		fmt.Printf("\n%s\n", names)
		if err != nil {
			return err
		}

		// Check to see if there are any logs base on the intake service name
		if len(names) == 0 {
			fmt.Println("found no log in service")
			return errors.New("no logs found in intake service")
		}

		// Check to see if service name matches
		service := "hello-world"
		logs, err := fakeintake.FilterLogs(service)
		if err != nil {
			return err
		}

		if len(logs) != 1 {
			m := fmt.Sprintf("no logs with service matching '%s' found, instead got '%s'", service, names)
			return errors.New(m)
		}

		// check if log's contain matches what is produced
		logs, err = fakeintake.FilterLogs("hello-world", fi.WithMessageContaining("hello-world"))

		if err != nil {
			return err
		}
		if len(logs) != 1 {
			return fmt.Errorf("received %v logs with 'hello-world', expecting 1", len(logs))
		}
		return nil

	}, backoff.WithMaxRetries(backoff.NewConstantBackOff(1*time.Second), 60))

	require.NoError(t, err)
}
