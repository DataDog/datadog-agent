package testinfradefinition

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/utils/clients"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/credentials"
	"github.com/DataDog/datadog-agent/test/new-e2e/utils/infra"
	"github.com/DataDog/test-infra-definitions/aws/scenarios/vm/agentinstall"
	"github.com/DataDog/test-infra-definitions/aws/scenarios/vm/ec2instance"
	"github.com/DataDog/test-infra-definitions/aws/scenarios/vm/os"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const pulumiStackName = "Test-EC2Instance"

func TestArchitecture(t *testing.T) {
	credentialsManager := credentials.NewManager()

	for _, data := range []struct {
		arch         os.Architecture
		expectedArch string
	}{
		{os.AMD64Arch, "amd64\n"},
		{os.ARM64Arch, "arm64\n"},
	} {
		client := runStack(t, credentialsManager, func(ctx *pulumi.Context) error {
			_, err := ec2instance.NewEc2Instance(ctx, ec2instance.WithOS(os.UbuntuOS, data.arch))
			return err
		})

		output, err := clients.ExecuteCommand(client, "dpkg --print-architecture")
		require.Equal(t, data.expectedArch, output)
		require.NoError(t, err)
	}
}

func TestVersion(t *testing.T) {
	credentialsManager := credentials.NewManager()
	apiKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.dev.apikey")
	require.NoError(t, err)
	for _, data := range []struct {
		version         string
		expectedVersion string
	}{
		{"7.41.0~rc.7-1", "7.41.0-rc.7"},
		{"6.39.0", "6.39.0"},
	} {
		client := runStack(t, credentialsManager, func(ctx *pulumi.Context) error {
			_, err := ec2instance.NewEc2Instance(ctx, ec2instance.WithHostAgent(apiKey, agentinstall.WithVersion(data.version)))
			return err
		})

		output, err := clients.ExecuteCommand(client, "datadog-agent version")
		require.NoError(t, err)
		require.True(t, strings.Contains(output, data.expectedVersion))
	}
}

func TestAgentConfig(t *testing.T) {
	credentialsManager := credentials.NewManager()
	apiKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.dev.apikey")
	require.NoError(t, err)
	for _, log_level := range []string{"log_level: info", "log_level: debug"} {
		client := runStack(t, credentialsManager, func(ctx *pulumi.Context) error {
			config := fmt.Sprintf("api_key: %%v\n%v", log_level)
			_, err := ec2instance.NewEc2Instance(ctx, ec2instance.WithHostAgent(apiKey, agentinstall.WithAgentConfig(config)))
			return err
		})

		output, err := clients.ExecuteCommand(client, "sudo datadog-agent config")
		require.NoError(t, err)
		require.True(t, strings.Contains(output, log_level))
	}
}

func runStack(t *testing.T, credentialsManager credentials.Manager, fct func(ctx *pulumi.Context) error) *ssh.Client {
	ctx := context.Background()
	stackOutput, err := infra.GetStackManager().GetStack(ctx, "aws/sandbox", pulumiStackName, nil, fct)
	require.NoError(t, err)

	instanceIP, found := stackOutput.Outputs["instance-ip"]
	require.True(t, found)
	ip := instanceIP.Value.(string)
	client, err := createUnixClient(credentialsManager, ip)
	require.NoError(t, err)
	return client
}

func createUnixClient(credentialsManager credentials.Manager, ip string) (*ssh.Client, error) {
	sshKey, err := credentialsManager.GetCredential(credentials.AWSSSMStore, "agent.ci.awssandbox.ssh")
	if err != nil {
		return nil, err
	}

	client, _, err := clients.GetSSHClient("ubuntu", fmt.Sprintf("%s:%d", ip, 22), sshKey, 2*time.Second, 5)
	if err != nil {
		return nil, err
	}
	return client, nil
}
