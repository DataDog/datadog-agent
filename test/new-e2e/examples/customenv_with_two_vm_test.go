// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	_ "embed"
	"os"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awsecs "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/ecs"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	compos "github.com/DataDog/test-infra-definitions/components/os"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"
	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed "testfixtures/http_compose.yaml"
var httpbinComposeContent string

type multiVMEnv struct {
	// This is initialised by the run function
	MainVM     *components.RemoteHost
	AppVM      *components.RemoteHost
	FakeIntake *components.FakeIntake
	// This is initialised by the Init function
	AppDocker *client.Docker
	Agent     *agentclient.Agent
}

// version one with explicit exports
func multiVMEnvProvisionerOne() e2e.PulumiEnvRunFunc[multiVMEnv] {
	return func(ctx *pulumi.Context, env *multiVMEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		// We create a first VM where we install a workload application using docker
		// We use os.AmazonLinuxECSDefault to have linux installed with docker
		appHost, err := ec2.NewVM(awsEnv, "app", ec2.WithOS(compos.AmazonLinuxECSDefault))
		if err != nil {
			return err
		}
		// we export the app vm to the suite's environment
		// this will export the remote host IP address, the username
		// the OS and other useful information
		appHost.Export(ctx, &env.AppVM.HostOutput)
		// initialize a docker manager without installing docker.io (that false)
		dockerManager, _, err := docker.NewManager(*awsEnv.CommonEnvironment, appHost, false)
		if err != nil {
			return err
		}
		composeContents := []docker.ComposeInlineManifest{docker.ComposeInlineManifest{
			Name:    "httpbin",
			Content: pulumi.String(httpbinComposeContent),
		}}
		_, err = dockerManager.ComposeStrUp("httpbin", composeContents, pulumi.StringMap{})
		if err != nil {
			return err
		}

		// we deploy a fakeintake
		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, "fakeintake")
		if err != nil {
			return err
		}
		err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)

		// Finally we create a second VM where we install the agent
		mainHost, err := ec2.NewVM(awsEnv, "main")
		if err != nil {
			return err
		}
		mainHost.Export(ctx, &env.MainVM.HostOutput)
		// we install the agent passing the reference to the fakeintake
		_, err = agent.NewHostAgent(awsEnv.CommonEnvironment, mainHost, agentparams.WithFakeintake(fakeintake))
		if err != nil {
			return err
		}

		return nil
	}
}

// version two with self-exporting components
func multiVMEnvProvisionerTwo() e2e.PulumiEnvRunFunc[multiVMEnv] {
	return func(ctx *pulumi.Context, env *multiVMEnv) error {
		// first, let's create an aws environment
		// this is where our remote hosts will live
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		// We create a first VM where we install a workload application using docker
		// We use os.AmazonLinuxECSDefault to have linux installed with docker
		appHost, err := awshost.NewHost(awshost.AwsHostParams{
			Name:           "app",
			PulumiContext:  ctx,
			AwsEnvironment: &awsEnv,
			Options:        []ec2.VMOption{ec2.WithOS(compos.AmazonLinuxECSDefault)},
			Importer:       &env.AppVM.HostOutput,
		})
		if err != nil {
			return err
		}

		// initialize a docker manager without installing docker.io (that false)
		dockerManager, _, err := docker.NewManager(*awsEnv.CommonEnvironment, appHost, false)
		if err != nil {
			return err
		}
		// we docker compose the httpbin application
		_, err = dockerManager.ComposeStrUp("httpbin", []docker.ComposeInlineManifest{{
			Name:    "httpbin",
			Content: pulumi.String(httpbinComposeContent),
		}}, pulumi.StringMap{})
		if err != nil {
			return err
		}

		// we deploy a fakeintake
		_, err = awsecs.NewFakeintake(awsecs.FakeintakeParams{
			Name:           "fakeintake",
			PulumiContext:  ctx,
			AwsEnvironment: &awsEnv,
			Importer:       &env.FakeIntake.FakeintakeOutput,
		})
		if err != nil {
			return err
		}

		// Finally we create a second VM where we install the agent
		mainHost, err := ec2.NewVM(awsEnv, "main")
		if err != nil {
			return err
		}
		mainHost.Export(ctx, &env.MainVM.HostOutput)
		// we install the agent on host
		_, err = agent.NewHostAgent(awsEnv.CommonEnvironment, mainHost)
		if err != nil {
			return err
		}

		return nil
	}
}

// Init is called by the suite to initialize the environment
// once pulumi is done provisioning the resources
// This is where we initialise clients
func (e *multiVMEnv) Init(ctx e2e.Context) error {
	privateKeyPath, err := runner.GetProfile().ParamStore().GetWithDefault(parameters.PrivateKeyPath, "")
	if err != nil {
		return err
	}

	e.AppDocker, err = client.NewDocker(ctx.T(), e.AppVM.HostOutput, privateKeyPath)
	if err != nil {
		return err
	}

	agent, err := client.NewHostAgentClient(ctx.T(), e.MainVM, true)
	if err != nil {
		return err
	}
	e.Agent = &agent
	return nil
}

type multiVMSuite struct {
	e2e.BaseSuite[multiVMEnv]
}

func TestMultiVMSuite(t *testing.T) {
	runFunc := multiVMEnvProvisionerOne
	if os.Getenv("USE_VARIANT_TWO") != "" {
		runFunc = multiVMEnvProvisionerTwo
	}
	e2e.Run(t, &multiVMSuite{}, e2e.WithPulumiProvisioner(runFunc(), nil))
}

func (v *multiVMSuite) TestItIsExpectedOS() {
	res := v.Env().MainVM.MustExecute("cat /etc/os-release")
	v.Assert().Contains(res, "Ubuntu")
	res = v.Env().AppVM.MustExecute("cat /etc/os-release")
	v.Assert().Contains(res, "Amazon Linux")
}
