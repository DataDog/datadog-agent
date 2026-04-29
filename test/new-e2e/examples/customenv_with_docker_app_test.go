// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	_ "embed"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/hostagent"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// vmPlusDockerEnv is a custom environment combining a remote host with Docker,
// a fakeintake, and a Datadog Agent. The Pulumi provisioner only sets up
// infrastructure (VM + fakeintake + docker manager). The agent and the
// lighttpd workload are deployed in SetupSuite, decoupled from Pulumi.
type vmPlusDockerEnv struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Fakeintake *components.FakeIntake
	Docker     *components.RemoteHostDocker
}

//go:embed testfixtures/docker-compose.lighttpd.yaml
var lighttpdComposeContent string

//go:embed testfixtures/lighttpd.conf
var lighttpdConfigContent string

//go:embed testfixtures/lighttpd_integration.conf.yaml
var lighttpdIntegrationConfigContent string

// vmPlusDockerInfraProvisioner sets up infrastructure only — VM + Docker manager
// + fakeintake. No agent and no workload (those are handled in SetupSuite).
func vmPlusDockerInfraProvisioner(ctx *pulumi.Context, env *vmPlusDockerEnv) error {
	// Mark Agent as not provisioned by Pulumi — it's installed in SetupSuite.
	// The framework auto-initializes all importable env fields to non-nil
	// zero values; setting it back to nil tells the framework to skip
	// resource import for this field.
	env.Agent = nil

	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	// Remote host with Amazon Linux ECS (Docker pre-installed)
	remoteHost, err := ec2.NewVM(awsEnv, "main", ec2.WithOS(os.AmazonLinuxECSDefault))
	if err != nil {
		return err
	}
	if err := remoteHost.Export(ctx, &env.RemoteHost.HostOutput); err != nil {
		return err
	}

	// Fakeintake on ECS Fargate
	fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, "")
	if err != nil {
		return err
	}
	if err := fakeIntake.Export(ctx, &env.Fakeintake.FakeintakeOutput); err != nil {
		return err
	}

	// Docker manager
	dockerManager, err := docker.NewAWSManager(&awsEnv, remoteHost)
	if err != nil {
		return err
	}
	if err := dockerManager.Export(ctx, &env.Docker.ManagerOutput); err != nil {
		return err
	}

	return nil
}

type vmPlusDockerEnvSuite struct {
	e2e.BaseSuite[vmPlusDockerEnv]
	remoteHostLogsDir string
}

func TestLighttpdOnDockerFromHost(t *testing.T) {
	e2e.Run(t, &vmPlusDockerEnvSuite{}, e2e.WithPulumiProvisioner(vmPlusDockerInfraProvisioner, nil))
}

func (v *vmPlusDockerEnvSuite) SetupSuite() {
	v.BaseSuite.SetupSuite()
	defer v.CleanupOnSetupFailure()

	host := v.Env().RemoteHost

	// Step 1: prepare a directory and config file for lighttpd on the host.
	tmpDir := strings.TrimSpace(host.MustExecute("mktemp -d /tmp/lighttpd.XXXXXX"))
	v.remoteHostLogsDir = tmpDir
	host.MustExecute(fmt.Sprintf("cat > %s/lighttpd.conf << 'CONFEOF'\n%s\nCONFEOF", tmpDir, lighttpdConfigContent))

	// Step 2: write the docker-compose file and start the lighttpd container.
	composePath := tmpDir + "/docker-compose.yaml"
	host.MustExecute(fmt.Sprintf("cat > %s << 'COMPOSEEOF'\n%s\nCOMPOSEEOF", composePath, lighttpdComposeContent))
	host.MustExecute(fmt.Sprintf(
		"cd %s && DD_LIGHTTPD_CONFIG=%s/lighttpd.conf DD_LIGHTTPD_LOGS_PATH=%s docker compose up -d",
		tmpDir, tmpDir, tmpDir,
	))

	// Step 3: install the Datadog agent. The fakeintake URL is wired in
	// automatically by InstallOnHost. The integration config references
	// the lighttpd log directory we just created.
	integrationConfig := strings.ReplaceAll(lighttpdIntegrationConfigContent, "LIGHTTPD_LOG_PATH", tmpDir)
	v.Env().Agent = hostagent.InstallOnHost(v.T(), v.Env().RemoteHost, v.Env().Fakeintake,
		agentparams.WithIntegration("lighttpd.d", integrationConfig),
		agentparams.WithLogs(),
	)
}

func (v *vmPlusDockerEnvSuite) TestListContainers() {
	containers, err := v.Env().Docker.Client.ListContainers()
	require.NoError(v.T(), err)
	assert.NotEmpty(v.T(), containers)
	assert.Contains(v.T(), containers, "lighttpd")
}

func (v *vmPlusDockerEnvSuite) TestAgentMonitorsLighttpd() {
	assert.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		logs, err := v.Env().Fakeintake.Client().FilterLogs("lighttpd")
		assert.NoError(c, err)
		assert.NotEmpty(c, logs)
	}, 5*time.Minute, 10*time.Second)
	assert.True(v.T(), v.Env().Agent.Client.IsReady())
	// ExecuteCommand executes a command on containerName and returns the output
	assert.NotEmpty(v.T(), v.Env().Docker.Client.ExecuteCommand("lighttpd", "ls"))
}
